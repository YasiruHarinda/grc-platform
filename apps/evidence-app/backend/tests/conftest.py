"""
Shared pytest fixtures for the backend test suite.

The app builds module-level state straight from the environment at import
time (`app/config.py`'s `Settings()`, `app/auth.py`'s Asgardeo userinfo URL,
`app/main.py`'s Azure `BlobServiceClient`), so every required setting must
already be in `os.environ` before `app.main` — or anything that imports it —
is imported for the first time anywhere in the test run. That's why the env
defaults below run before any `app.*` import in this file.

Tests exercise the FastAPI app over HTTP via `TestClient`, with two
dependencies overridden so no real Asgardeo account is needed and no fake
stands in for Azure Blob Storage:

- `get_current_user` -> a fake Engineer or Admin identity (`engineer_client`
  / `admin_client` fixtures below).
- `get_db` -> a session on a throwaway Postgres database, wrapped in a
  transaction that's rolled back after every test for per-test isolation
  (`db_session` fixture below).

Blob storage is *not* faked or overridden: routes that upload, sign or
delete files talk to a real Azurite emulator, exactly as they would talk to
real Azure in production (see the `blob_container` fixture below). This
follows the same reasoning as choosing Postgres over SQLite — tests then
assert the real property ("the file is gone") rather than an internal one
("we called delete_file").

Postgres was chosen over SQLite so Postgres-specific column types (e.g.
`DateTime(timezone=True)`, `JSON`) behave the same in tests as in production
— see issue #6/#7. A real Postgres must be reachable at `TEST_DATABASE_URL`
before running the suite, e.g.:

    docker run -d --name evidence-app-test-db \\
        -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres \\
        -e POSTGRES_DB=evidence_app_test -p 5433:5432 postgres:16

A real Azurite blob-storage emulator must also be reachable, at the host and
port baked into `AZURE_STORAGE_CONNECTION_STRING` below (`127.0.0.1:10000`
by default), e.g.:

    docker run -d --name evidence-app-test-blob -p 10000:10000 \\
        mcr.microsoft.com/azure-storage/azurite:latest \\
        azurite-blob --blobHost 0.0.0.0 --skipApiVersionCheck
"""
import os

TEST_DATABASE_URL = os.environ.get(
    "TEST_DATABASE_URL",
    "postgresql://postgres:postgres@localhost:5433/evidence_app_test",
)

os.environ.setdefault("DATABASE_URL", TEST_DATABASE_URL)
# The well-known public Azurite emulator credential — not a secret. No
# network call happens at import time; `blob_container` below is what
# actually dials the emulator, once per session, and fails the run loudly if
# it isn't reachable.
AZURE_BLOB_EMULATOR_ENDPOINT = "127.0.0.1:10000"
os.environ.setdefault(
    "AZURE_STORAGE_CONNECTION_STRING",
    "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;"
    "AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;"
    f"BlobEndpoint=http://{AZURE_BLOB_EMULATOR_ENDPOINT}/devstoreaccount1;",
)
# Required: app/config.py has no default for this setting, so it must be in
# the environment before anything imports app.config for the first time.
os.environ.setdefault("ASGARDEO_ORG", "test-org")

import io

import pytest
from azure.core.exceptions import ResourceExistsError
from azure.storage.blob import BlobServiceClient
from fastapi import UploadFile
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, event
from sqlalchemy.engine import make_url
from sqlalchemy.orm import Session
from starlette.datastructures import Headers

from app.auth import User, get_current_user
from app.config import settings
from app.database import Base, get_db
from app.main import app
from app.models.control import Control
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.models.framework import Framework
from app.models.product import Product
from app.storage.blob_storage import save_file

# Import every model so Base.metadata knows about all tables — mirrors the
# import list in alembic/env.py, which needs the same thing for autogenerate.
from app.models import (  # noqa: F401
    agent_task,
    control,
    evidence,
    evidence_file,
    framework,
    product,
    submission,
    usage_log,
)


def _parse_connection_string(conn_str: str) -> dict[str, str]:
    """Split an Azure storage connection string into its `key=value;` parts.

    Values can themselves contain "=" (account keys are base64), so each part
    is split on the *first* "=" only.
    """
    parts = {}
    for part in conn_str.split(";"):
        if "=" in part:
            key, value = part.split("=", 1)
            parts[key.strip()] = value.strip()
    return parts


def _blob_endpoint(conn_str: str) -> str:
    """The endpoint the storage client will actually dial, for error messages.

    Never report a hardcoded guess: the connection string is read from the
    environment and may point somewhere entirely different from the emulator
    default, which is exactly the case an error most needs to be honest about.
    """
    parts = _parse_connection_string(conn_str)
    return parts.get("BlobEndpoint") or parts.get("AccountName") or "<unknown endpoint>"


def _is_emulator_storage(conn_str: str) -> bool:
    """True only when `conn_str` explicitly names the local Azurite emulator.

    The blob-storage counterpart of `_is_test_database` below, and it exists
    for the same reason. `AZURE_STORAGE_CONNECTION_STRING` is read from the
    environment with `os.environ.setdefault`, so an existing value — a
    developer's real one, exported to run the app — wins over the emulator
    default. Without this guard the suite would upload test blobs to, and
    delete blobs from, whatever storage that names, and `blob_container`'s
    teardown would wipe it.

    Keyed on the account name being exactly the emulator's well-known
    `devstoreaccount1`, which is what identifies the target, rather than on
    the endpoint host — so Azurite reachable at some other host (a CI service
    container, say) still works. As with `_is_test_database`, this rejects an
    unconventionally-configured real emulator rather than risk accepting real
    storage: "refuses to run" is the safe failure mode.
    """
    return _parse_connection_string(conn_str).get("AccountName") == "devstoreaccount1"


def _is_test_database(url: str) -> bool:
    """True only when the database *name* in `url` explicitly marks it as a
    disposable test database: exactly "test", or ending in "_test"
    (case-insensitive).

    Deliberately looks at nothing but the database name — not the host,
    user, or the URL string as a whole — because that's the one part of the
    URL that names what `CREATE TABLE` / `DROP TABLE` actually runs against.
    A host like "test-replica.prod.internal" or a database named
    "attestation_log" both contain the letters "test" in a misleading
    position; neither should be able to satisfy this guard. Requiring an
    exact match or a "_test" suffix (not a bare substring) is what rules
    those out, at the cost of also rejecting a real test database that
    happens to be named unconventionally — a deliberate trade in favour of
    "refuses to run" being the safe failure mode.
    """
    name = (make_url(url).database or "").lower()
    return name == "test" or name.endswith("_test")


@pytest.fixture(scope="session")
def db_engine():
    """Session-scoped engine against the throwaway test Postgres. Creates
    every table from the SQLAlchemy models once for the whole run and drops
    them at the end; per-test isolation is handled by `db_session` below."""
    if not _is_test_database(TEST_DATABASE_URL):
        pytest.exit(
            "Refusing to run destructive test setup (CREATE TABLE / DROP "
            f"TABLE) against {TEST_DATABASE_URL!r}: its database name is "
            "not explicitly a test database (must be exactly \"test\", or "
            "end with \"_test\").\n"
            "Point TEST_DATABASE_URL at a throwaway database instead, e.g.:\n"
            "  postgresql://postgres:postgres@localhost:5433/evidence_app_test",
            returncode=1,
        )

    engine = create_engine(TEST_DATABASE_URL)
    try:
        with engine.connect():
            pass
    except Exception as exc:
        pytest.exit(
            "Could not reach the throwaway test Postgres at "
            f"{TEST_DATABASE_URL!r} ({exc!r}).\n"
            "Start one, e.g.:\n"
            "  docker run -d --name evidence-app-test-db "
            "-e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres "
            "-e POSTGRES_DB=evidence_app_test -p 5433:5432 postgres:16",
            returncode=1,
        )

    Base.metadata.create_all(engine)
    try:
        yield engine
    finally:
        Base.metadata.drop_all(engine)
        engine.dispose()


@pytest.fixture(scope="session", autouse=True)
def blob_container():
    """Session-scoped, autouse: proves the Azurite blob-storage emulator is
    reachable and creates the storage container once for the whole run —
    mirroring `db_engine` above, both in what it checks (a real dependency
    the suite needs) and how it fails (immediately, naming the endpoint and
    how to start it).

    Autouse so this happens unconditionally at the start of the run, exactly
    like `db_engine`'s connectivity check — a test that doesn't explicitly
    ask for a storage fixture still needs the container to exist before any
    route under test tries to upload to it.

    Creating the container is the one "destructive-ish" setup step blob
    storage needs (parallel to `Base.metadata.create_all` for Postgres), so
    it must be idempotent across repeated runs of the suite: a container
    that already exists from a previous run is not an error.

    Teardown deletes every blob the run left behind, mirroring `db_engine`'s
    `drop_all` — without it, each run strands the blobs its uploads created
    (the database rows roll back, the blobs do not), and they accumulate in
    the emulator forever. The blobs are removed rather than the container
    itself: deleting a container makes the name unusable until Azure finishes
    reclaiming it, which would make consecutive runs flaky for no gain.
    """
    conn_str = settings.AZURE_STORAGE_CONNECTION_STRING
    endpoint = _blob_endpoint(conn_str)

    if not _is_emulator_storage(conn_str):
        pytest.exit(
            "Refusing to run blob-storage tests against "
            f"{endpoint!r}: it is not the local Azurite emulator (its account "
            'name is not "devstoreaccount1"). This suite uploads and deletes '
            "real blobs, and clears the container at teardown, so it must "
            "never point at real storage.\n"
            "AZURE_STORAGE_CONNECTION_STRING is almost certainly set in your "
            "environment — unset it, and the emulator default in "
            "tests/conftest.py will be used instead.",
            returncode=1,
        )

    # retry_total=0: the SDK's default retry policy backs off for over a
    # minute before surfacing a connection error, which would make a missing
    # emulator look like a hung suite rather than a fast, clear failure.
    # Nothing here is worth retrying — either the emulator is up or it isn't.
    service = BlobServiceClient.from_connection_string(
        conn_str, retry_total=0, connection_timeout=5
    )
    try:
        service.create_container(settings.AZURE_STORAGE_CONTAINER)
    except ResourceExistsError:
        pass
    except Exception as exc:
        pytest.exit(
            "Could not reach the Azurite blob-storage emulator at "
            f"{endpoint!r} ({exc!r}).\n"
            "Start one, e.g.:\n"
            "  docker run -d --name evidence-app-test-blob -p 10000:10000 \\\n"
            "      mcr.microsoft.com/azure-storage/azurite:latest \\\n"
            "      azurite-blob --blobHost 0.0.0.0 --skipApiVersionCheck",
            returncode=1,
        )

    try:
        yield
    finally:
        container = service.get_container_client(settings.AZURE_STORAGE_CONTAINER)
        for blob in container.list_blobs():
            container.delete_blob(blob.name)


@pytest.fixture()
def db_session(db_engine):
    """One Postgres session per test.

    The session is bound to a connection holding an outer transaction plus a
    SAVEPOINT, and both are rolled back at teardown — so nothing a test does
    is ever visible to another test, even though route handlers call
    `db.commit()` themselves (some, like deleting the last Evidence File,
    call it more than once per request). This is the standard SQLAlchemy
    pattern for joining a Session to an external transaction in a test suite.
    """
    connection = db_engine.connect()
    outer_transaction = connection.begin()
    session = Session(bind=connection)

    nested = connection.begin_nested()

    @event.listens_for(session, "after_transaction_end")
    def _restart_savepoint(session, transaction):
        nonlocal nested
        if not nested.is_active:
            nested = connection.begin_nested()

    try:
        yield session
    finally:
        session.close()
        outer_transaction.rollback()
        connection.close()


@pytest.fixture()
def engineer_user() -> User:
    return User(email="engineer@example.com", role="engineer")


@pytest.fixture()
def admin_user() -> User:
    return User(email="admin@example.com", role="admin")


@pytest.fixture()
def client(db_session):
    """A TestClient with only `get_db` overridden — `get_current_user` is
    left wired to the real dependency, so requests without a Bearer token
    still get a real 401. Useful for auth-boundary tests; most tests want
    `engineer_client` or `admin_client` instead."""
    app.dependency_overrides[get_db] = lambda: db_session
    try:
        yield TestClient(app)
    finally:
        app.dependency_overrides.pop(get_db, None)


def _client_as(db_session, user: User):
    app.dependency_overrides[get_db] = lambda: db_session
    app.dependency_overrides[get_current_user] = lambda: user
    try:
        yield TestClient(app)
    finally:
        app.dependency_overrides.pop(get_db, None)
        app.dependency_overrides.pop(get_current_user, None)


@pytest.fixture()
def engineer_client(db_session, engineer_user):
    """A TestClient with `get_db` pointed at the isolated test session and
    `get_current_user` faked as an Engineer — no real Asgardeo call."""
    yield from _client_as(db_session, engineer_user)


@pytest.fixture()
def admin_client(db_session, admin_user):
    """Same as `engineer_client`, but faked as an Admin."""
    yield from _client_as(db_session, admin_user)


# --- Shared test helpers -----------------------------------------------
#
# Used by more than one test module (originally written for
# test_evidence_creation.py and test_evidence_file_deletion.py; `upload_blob`
# and `build_evidence` were promoted here from test_evidence_file_deletion.py
# once test_cascade_evidence_file_deletion.py needed the same multi-file
# Evidence construction too), so they live here rather than being
# copy-pasted between test modules.


def uploaded_blob_names() -> set[str]:
    """The names of every blob currently sitting in the test container —
    used to assert an upload really did or did not survive a request, by
    diffing this set before and after, rather than trusting that some
    internal function was called."""
    container = BlobServiceClient.from_connection_string(
        settings.AZURE_STORAGE_CONNECTION_STRING
    ).get_container_client(settings.AZURE_STORAGE_CONTAINER)
    return {blob.name for blob in container.list_blobs()}


def make_control(db_session) -> Control:
    """A minimal Product/Framework/Control chain, for tests that need a
    real control_id to attach Evidence to."""
    product = Product(name="Test Product")
    db_session.add(product)
    db_session.flush()
    framework = Framework(product_id=product.id, name="Test Framework")
    db_session.add(framework)
    db_session.flush()
    control = Control(framework_id=framework.id, control_ref="C-1", title="Test Control")
    db_session.add(control)
    db_session.commit()
    db_session.refresh(control)
    return control


def upload_blob(name: str, content: bytes) -> tuple[str, str]:
    """Puts a real blob into the test container and returns
    (file_name, file_url) — what `save_file` returns for a real upload.

    Sets a real image `Content-Type` header: `save_file` allow-lists image
    content types, and every real caller of this helper is standing in for
    an actual image upload, so this matches what a genuine request would
    put on the wire rather than weakening the allow-list to fit the test."""
    return save_file(
        UploadFile(
            file=io.BytesIO(content),
            filename=name,
            headers=Headers({"content-type": "image/png"}),
        )
    )


def build_evidence(
    db_session,
    *files: tuple[str, bytes],
    control_id: int | None = None,
    created_by: str = "engineer@example.com",
) -> tuple[Evidence, list[EvidenceFile]]:
    """An Evidence whose primary reference and Evidence File list both point
    at the first upload, followed by one EvidenceFile per remaining upload,
    in presentation order (ascending sort_order) — the shape both
    `create_evidence` and the AI-agent result path produce.

    `control_id` is optional so callers that only need a bare multi-file
    Evidence (e.g. Evidence File deletion tests) don't have to build a
    Control chain they don't otherwise need; cascade tests pass the id of a
    real Control (see `make_control` above) so the Evidence is actually
    reachable via product -> frameworks -> controls -> evidence."""
    uploads = [upload_blob(name, content) for name, content in files]
    primary_name, primary_url = uploads[0]

    evidence = Evidence(
        title="Console screenshot",
        file_name=primary_name,
        file_url=primary_url,
        control_id=control_id,
        created_by=created_by,
    )
    db_session.add(evidence)
    db_session.flush()

    evidence_files = [
        EvidenceFile(evidence_id=evidence.id, file_name=file_name, file_url=file_url, sort_order=i)
        for i, (file_name, file_url) in enumerate(uploads)
    ]
    for ef in evidence_files:
        db_session.add(ef)
    db_session.commit()

    db_session.refresh(evidence)
    for ef in evidence_files:
        db_session.refresh(ef)

    return evidence, evidence_files
