"""
Coverage for the size and content-type guards `save_file` enforces before
ever writing a blob (see `app/storage/blob_storage.py`).

Driven over HTTP through `POST /api/evidence` — the Engineer upload path —
following the same real-emulator, real-property style as
`test_evidence_creation.py`: these assert what a caller would actually see
(the status code, and whether a blob really exists in the container) rather
than whether some internal function was called.
"""
import httpx

from app.models.evidence import Evidence
from app.storage.blob_storage import MAX_UPLOAD_SIZE_BYTES

from tests.conftest import make_control, uploaded_blob_names


def test_upload_over_the_size_limit_is_rejected_and_writes_no_blob(
    db_session, engineer_client
):
    """One byte over the cap must be rejected with a client error, and the
    request must never reach blob storage — checked by diffing the
    container's real contents, not by trusting the response alone."""
    control = make_control(db_session)
    blobs_before = uploaded_blob_names()
    oversized = b"x" * (MAX_UPLOAD_SIZE_BYTES + 1)

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("too-big.png", oversized, "image/png")},
    )

    assert response.status_code == 413

    assert db_session.query(Evidence).count() == 0
    assert uploaded_blob_names() == blobs_before


def test_upload_with_disallowed_content_type_is_rejected_and_writes_no_blob(
    db_session, engineer_client
):
    """A content type outside the image allow-list is rejected with a
    client error before any blob is written -- an allow-list, not a
    deny-list, so this covers any type this app doesn't expect, not just a
    hand-picked "dangerous" one."""
    control = make_control(db_session)
    blobs_before = uploaded_blob_names()

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("not-an-image.exe", b"MZ not an image", "application/octet-stream")},
    )

    assert response.status_code == 400

    assert db_session.query(Evidence).count() == 0
    assert uploaded_blob_names() == blobs_before


def test_upload_within_the_size_limit_succeeds_end_to_end(db_session, engineer_client):
    """An allowed image type, sized right up against the cap without going
    over it, still succeeds -- the guards reject what they should and
    nothing else. Fetched back from the real emulator, the same way
    `test_evidence_creation.py` verifies a normal upload really landed."""
    control = make_control(db_session)
    content = b"y" * (MAX_UPLOAD_SIZE_BYTES - 1024)

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console screenshot", "control_id": str(control.id)},
        files={"file": ("big-but-ok.png", content, "image/png")},
    )

    assert response.status_code == 201

    fetch = httpx.get(response.json()["file_url"])
    assert fetch.status_code == 200
    assert fetch.content == content


def test_upload_with_pdf_content_type_is_accepted_end_to_end(db_session, engineer_client):
    """`application/pdf` is on the allow-list alongside the image types --
    the Runner uploads PDFs, not just screenshots -- so a PDF upload
    succeeds and is really retrievable from the emulator, the same way the
    image case above is verified."""
    control = make_control(db_session)
    content = b"%PDF-1.4 not a real pdf but bytes are bytes"

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Audit report", "control_id": str(control.id)},
        files={"file": ("report.pdf", content, "application/pdf")},
    )

    assert response.status_code == 201

    fetch = httpx.get(response.json()["file_url"])
    assert fetch.status_code == 200
    assert fetch.content == content


def test_upload_with_still_disallowed_content_type_is_rejected_with_allowed_types_message(
    db_session, engineer_client
):
    """Adding `application/pdf` to the allow-list must not turn it into a
    deny-list: a type that was never allowed (e.g. a zip archive) is still
    rejected with a 400 that names the currently allowed types, generated
    live from `ALLOWED_UPLOAD_CONTENT_TYPES` rather than hard-coded."""
    control = make_control(db_session)
    blobs_before = uploaded_blob_names()

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Archive", "control_id": str(control.id)},
        files={"file": ("bundle.zip", b"PK not a real zip", "application/zip")},
    )

    assert response.status_code == 400
    assert "application/pdf" in response.json()["detail"]

    assert db_session.query(Evidence).count() == 0
    assert uploaded_blob_names() == blobs_before
