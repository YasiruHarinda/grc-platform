"""
End-to-end coverage of `POST /api/evidence` — the app's main write path,
never exercised by any test until now because it uploads to blob storage on
its first line and the suite had no seam for blob storage at all (see
`tests/conftest.py`'s `blob_container` fixture, added alongside this file).

These run against a real Azurite emulator, so they assert what a user of the
API would actually observe — the response, the database rows, and whether
an uploaded file can really be fetched or is really gone after deletion —
never an internal detail like "we called save_file" / "we called
delete_file". That distinction is the reason a real emulator was chosen
over a fake in the first place.
"""
import httpx

from app.models.evidence_file import EvidenceFile
from app.models.evidence import Evidence
from app.models.submission import Submission
from app.storage.blob_storage import get_signed_url

from tests.conftest import make_control, uploaded_blob_names


def _evidence_request_with_raw_filename(fields: dict[str, str], filename: str, content: bytes) -> tuple[bytes, str]:
    """Builds a raw `multipart/form-data` body for `POST /api/evidence` with
    the file part's `filename` attribute set to exactly `filename`,
    including the empty string.

    `httpx`'s own `files=` parameter can't produce this: it omits the
    `filename` attribute entirely for any falsy value, which changes what's
    being sent on the wire — Starlette then treats the part as a plain
    string field rather than an upload, and FastAPI rejects it with its own
    422 before the request ever reaches `create_evidence`. Sending an
    explicit but empty `filename=""` is what a real non-browser client
    (the Runner, curl, a script) with a filename-handling bug would
    actually put on the wire, and it is what reaches `save_file`.
    """
    boundary = "----EvidenceAppTestBoundary"
    parts = [
        f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'
        for name, value in fields.items()
    ]
    parts.append(
        f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\n'
        f'Content-Type: image/png\r\n\r\n'
    )
    body = "".join(parts).encode() + content + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


def test_create_evidence_end_to_end(db_session, engineer_client, engineer_user):
    """The request succeeds, the Evidence, its Evidence File and its
    Submission all exist, and the uploaded file is really fetchable —
    a real GET against the emulator, not an assertion that some internal
    upload function was called."""
    control = make_control(db_session)

    response = engineer_client.post(
        "/api/evidence",
        data={
            "title": "Console screenshot",
            "control_id": str(control.id),
            "description": "proof the control is satisfied",
        },
        files={"file": ("screenshot.png", b"fake screenshot bytes", "image/png")},
    )

    assert response.status_code == 201
    body = response.json()

    evidence = db_session.query(Evidence).filter(Evidence.id == body["id"]).one()
    assert evidence.title == "Console screenshot"
    assert evidence.control_id == control.id
    assert evidence.created_by == engineer_user.email

    evidence_file = (
        db_session.query(EvidenceFile)
        .filter(EvidenceFile.evidence_id == evidence.id)
        .one()
    )
    assert evidence_file.file_name == evidence.file_name

    submission = (
        db_session.query(Submission).filter(Submission.evidence_id == evidence.id).one()
    )
    assert submission.status == "pending"
    assert submission.submitted_by == engineer_user.email

    fetch = httpx.get(body["file_url"])
    assert fetch.status_code == 200
    assert fetch.content == b"fake screenshot bytes"


def test_deleting_evidence_file_really_removes_it_from_storage(
    db_session, engineer_client, admin_client
):
    """A later fetch of a deleted file's blob fails rather than succeeding —
    the property that matters, not whether `delete_file` was invoked."""
    control = make_control(db_session)

    create_response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("screenshot.png", b"more fake bytes", "image/png")},
    )
    assert create_response.status_code == 201
    evidence_id = create_response.json()["id"]

    evidence_file = (
        db_session.query(EvidenceFile)
        .filter(EvidenceFile.evidence_id == evidence_id)
        .one()
    )

    # Fetchable before deletion — establishes the file was really there.
    assert httpx.get(get_signed_url(evidence_file.file_name)).status_code == 200

    delete_response = admin_client.delete(f"/api/evidence/files/{evidence_file.id}")
    assert delete_response.status_code == 204

    fetch_after = httpx.get(get_signed_url(evidence_file.file_name))
    assert fetch_after.status_code == 404


def _fail_partway(monkeypatch) -> None:
    """Make the write blow up after the upload has happened and the Evidence
    and its Evidence File are already in the transaction — the exact window
    that used to leave a half-built record behind.

    Monkeypatching stands in for a database going away mid-write, which can't
    be provoked on demand against a real Postgres. It patches the route's own
    reference, so it adds no seam to production code. It deliberately does
    *not* fake blob storage: the upload, the cleanup and the assertions below
    all run against the real emulator.
    """
    def _boom(*args, **kwargs):
        raise RuntimeError("database went away mid-write")

    monkeypatch.setattr("app.api.routes.evidence.Submission", _boom)


def test_create_evidence_with_an_unknown_control_is_a_bad_request_not_a_server_error(
    db_session, engineer_client
):
    """Naming a Control that doesn't exist is the caller's mistake, so it must
    come back as not-found. Left to the foreign key it would surface as a 500
    inviting a retry that can never succeed.

    The parent is also resolved before the upload, so a doomed request never
    puts a blob into storage that would then need cleaning up.
    """
    blobs_before = uploaded_blob_names()

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": "999999999"},
        files={"file": ("screenshot.png", b"never uploaded", "image/png")},
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Control not found"

    assert db_session.query(Evidence).count() == 0
    assert uploaded_blob_names() == blobs_before


def test_evidence_creation_leaves_nothing_behind_when_the_write_fails(
    db_session, engineer_client, monkeypatch
):
    """A partial failure must leave the database and storage exactly as if
    the request had never been made.

    Asserts only observable state: no Evidence, Evidence File or Submission
    row survives, and the blob uploaded before the failing write does not
    survive either — diffed against the container's real contents, never
    "delete_file was called".
    """
    control = make_control(db_session)
    blobs_before = uploaded_blob_names()
    _fail_partway(monkeypatch)

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("screenshot.png", b"orphan bait", "image/png")},
    )

    assert response.status_code == 500

    assert db_session.query(Evidence).count() == 0
    assert db_session.query(EvidenceFile).count() == 0
    assert db_session.query(Submission).count() == 0

    assert uploaded_blob_names() == blobs_before


def test_retry_after_a_failed_creation_produces_exactly_one_record(
    db_session, engineer_client, monkeypatch
):
    """A retry after a failure must not compete with debris from the first
    attempt: exactly one complete Evidence/Evidence File/Submission, and a
    file that really is fetchable — not one complete record and one broken."""
    control = make_control(db_session)

    with monkeypatch.context() as failing:
        _fail_partway(failing)
        failed_response = engineer_client.post(
            "/api/evidence",
            data={"title": "Console screenshot", "control_id": str(control.id)},
            files={"file": ("screenshot.png", b"first attempt, fails", "image/png")},
        )
    assert failed_response.status_code == 500

    retry_response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("screenshot.png", b"second attempt, succeeds", "image/png")},
    )
    assert retry_response.status_code == 201

    assert db_session.query(Evidence).count() == 1
    assert db_session.query(EvidenceFile).count() == 1
    assert db_session.query(Submission).count() == 1

    fetch = httpx.get(retry_response.json()["file_url"])
    assert fetch.status_code == 200
    assert fetch.content == b"second attempt, succeeds"

    assert db_session.query(Evidence).count() == 1
    assert db_session.query(EvidenceFile).count() == 1
    assert db_session.query(Submission).count() == 1

    fetch = httpx.get(retry_response.json()["file_url"])
    assert fetch.status_code == 200
    assert fetch.content == b"second attempt, succeeds"


def test_create_evidence_with_no_filename_is_a_bad_request(db_session, engineer_client):
    """A caller that uploads a file with no name at all gets a clear,
    actionable client error rather than an opaque server error, and nothing
    is uploaded to storage for a request that was rejected before any
    upload happened.

    A real browser file input always attaches a name, so this only shows up
    from a non-browser client (the Runner, curl, a script) that built its
    request incorrectly — an empty `filename` on the wire, sent explicitly
    via `_evidence_request_with_raw_filename` since `httpx`'s own `files=`
    can't produce it (see that helper's docstring).
    """
    control = make_control(db_session)
    blobs_before = uploaded_blob_names()
    body, content_type = _evidence_request_with_raw_filename(
        {"title": "Console screenshot", "control_id": str(control.id)},
        filename="",
        content=b"nameless bytes",
    )

    response = engineer_client.post(
        "/api/evidence",
        content=body,
        headers={"Content-Type": content_type},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Uploaded file must have a filename"

    assert db_session.query(Evidence).count() == 0
    assert db_session.query(EvidenceFile).count() == 0
    assert db_session.query(Submission).count() == 0
    assert uploaded_blob_names() == blobs_before


def test_create_evidence_with_an_unusual_but_present_filename_is_unaffected(
    db_session, engineer_client
):
    """A present filename that just happens to be unusual — no extension,
    or a trailing dot — is a real name the client chose, not a missing one.
    It is stored and served exactly like any other upload, unaffected by
    the no-filename rejection covered above."""
    control = make_control(db_session)

    for filename, content in (
        ("screenshot", b"no extension in the filename"),
        ("screenshot.", b"trailing dot in the filename"),
    ):
        response = engineer_client.post(
            "/api/evidence",
            data={"title": "Console screenshot", "control_id": str(control.id)},
            files={"file": (filename, content, "image/png")},
        )

        assert response.status_code == 201
        fetch = httpx.get(response.json()["file_url"])
        assert fetch.status_code == 200
        assert fetch.content == content
