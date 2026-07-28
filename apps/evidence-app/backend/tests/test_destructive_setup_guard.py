"""
`tests/conftest.py`'s `db_engine` fixture runs destructive setup — it
`CREATE TABLE`s and, at teardown, `DROP TABLE`s every model on whatever
database `TEST_DATABASE_URL` names. A mis-set environment variable could
point that at a real database, so `db_engine` refuses to run unless the
database name explicitly marks it as disposable (see `_is_test_database`).

Two things are covered:

- `_is_test_database` directly, including the cases the guard exists to
  reject (a "test"-looking host or a database name where "test" appears in
  a misleading position) — this is the one case that can't be produced
  through the normal HTTP/fixture seam, the same reasoning
  `test_evidence_ownership.py` gives for unit-testing
  `_authorize_evidence_access` directly.
- The guard actually firing, end to end, in a subprocess: the real,
  observable behaviour is that a pytest run refuses to proceed and says why,
  not that some internal function returned `False`. This can't be exercised
  in-process because the guard aborts the whole test session
  (`pytest.exit`), so a subprocess is the only seam available for the real
  property.
"""
import os
import subprocess
import sys
from pathlib import Path

from tests.conftest import _blob_endpoint, _is_emulator_storage, _is_test_database

BACKEND_DIR = Path(__file__).resolve().parent.parent

EMULATOR_CONN = (
    "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;"
    "AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;"
    "BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
)
REAL_AZURE_CONN = (
    "DefaultEndpointsProtocol=https;AccountName=complianceevidence;"
    "AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;"
    "BlobEndpoint=https://complianceevidence.blob.core.windows.net;"
)


def test_exact_name_test_is_allowed():
    assert _is_test_database("postgresql://user:pw@localhost:5433/test") is True


def test_name_ending_in_underscore_test_is_allowed():
    assert _is_test_database("postgresql://user:pw@localhost:5433/evidence_app_test") is True


def test_name_check_is_case_insensitive():
    assert _is_test_database("postgresql://user:pw@localhost:5433/Evidence_App_TEST") is True


def test_misleading_hostname_does_not_satisfy_the_guard():
    """"test" in the host, not the database name, must not count — the host
    is not what CREATE TABLE / DROP TABLE runs against."""
    assert _is_test_database("postgresql://user:pw@test-replica.prod.internal:5432/production") is False


def test_name_containing_test_as_a_substring_does_not_satisfy_the_guard():
    """"test" appearing inside another word, not as an explicit "_test"
    suffix, must not count."""
    assert _is_test_database("postgresql://user:pw@localhost:5432/attestation_log") is False
    assert _is_test_database("postgresql://user:pw@localhost:5432/contest_entries") is False


def test_name_with_test_as_a_prefix_only_does_not_satisfy_the_guard():
    """A name that merely starts with "test_" but goes on to something else
    (e.g. genuinely production data staged under a misleading name) is not
    an explicit test database either — only an exact match or a "_test"
    suffix is."""
    assert _is_test_database("postgresql://user:pw@localhost:5432/test_of_production_mirror") is False


def test_emulator_connection_string_is_allowed():
    assert _is_emulator_storage(EMULATOR_CONN) is True


def test_real_storage_account_does_not_satisfy_the_storage_guard():
    """The case the storage guard exists for. `AZURE_STORAGE_CONNECTION_STRING`
    is read with `os.environ.setdefault`, so a developer's exported real value
    wins over the emulator default — and this suite uploads blobs, deletes
    blobs, and clears the container at teardown."""
    assert _is_emulator_storage(REAL_AZURE_CONN) is False


def test_real_account_merely_containing_the_emulator_name_does_not_satisfy_it():
    """An exact account-name match, not a substring — the same reasoning as
    `_is_test_database` requiring an exact name or a "_test" suffix."""
    conn = REAL_AZURE_CONN.replace("complianceevidence", "devstoreaccount1backup")
    assert _is_emulator_storage(conn) is False


def test_storage_guard_is_not_fooled_by_a_loopback_endpoint_on_a_real_account():
    """A real account name is disqualifying even when the endpoint looks
    local — a tunnel or a proxy to real storage must not slip through."""
    conn = (
        "DefaultEndpointsProtocol=http;AccountName=complianceevidence;"
        "AccountKey=x==;BlobEndpoint=http://127.0.0.1:10000/complianceevidence;"
    )
    assert _is_emulator_storage(conn) is False


def test_blob_endpoint_reports_the_real_target_not_a_hardcoded_default():
    """Error messages must name where the client will actually dial. Reporting
    a hardcoded emulator address would tell a developer the emulator is down
    while the client was really talking to Azure."""
    assert _blob_endpoint(REAL_AZURE_CONN) == "https://complianceevidence.blob.core.windows.net"
    assert _blob_endpoint(EMULATOR_CONN) == "http://127.0.0.1:10000/devstoreaccount1"


def test_connection_string_parsing_survives_base64_padding_in_the_key():
    """Account keys are base64 and contain "=", so parts must split on the
    first "=" only — a naive split would corrupt every field after the key."""
    assert _is_emulator_storage(EMULATOR_CONN) is True
    assert _blob_endpoint(EMULATOR_CONN) == "http://127.0.0.1:10000/devstoreaccount1"


def test_destructive_setup_refuses_and_says_why_for_a_non_test_database():
    """End-to-end: run a real pytest session against a database URL whose
    name is plausible-looking but not explicitly a test database, and prove
    the whole run refuses immediately, naming the database it refused and
    why — before ever attempting to connect to it. (The host:port here is
    the real test Postgres container, but the guard must reject on the name
    alone, without dialing out, so this is safe to run.)
    """
    env = {k: v for k, v in os.environ.items() if k != "DATABASE_URL"}
    env["TEST_DATABASE_URL"] = "postgresql://postgres:postgres@localhost:5433/evidence_app_prod"

    result = subprocess.run(
        [sys.executable, "-m", "pytest", "tests/test_db_isolation.py", "-q"],
        cwd=BACKEND_DIR,
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )

    output = result.stdout + result.stderr
    assert result.returncode != 0, output
    assert "evidence_app_prod" in output
    assert "not explicitly a test database" in output
