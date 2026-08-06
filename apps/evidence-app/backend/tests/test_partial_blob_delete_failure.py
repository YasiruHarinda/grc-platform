"""
Coverage for `delete_files` (`app/storage/blob_storage.py`), the shared
per-file blob-deletion step every delete route calls after its database
commit has already succeeded.

Before this fix, each route looped calling `delete_file` directly:
`for name in file_names: delete_file(name)`. `delete_file` only swallows
`ResourceNotFoundError` -- any other failure (a network blip, throttling, a
transient auth problem) propagated straight out of the route. Because the
database commit had already happened by that point, the practical effect
was: the delete had genuinely succeeded, the caller was told it failed (a
500), and the loop stopped at the first error, abandoning every file after
it -- not just the one that failed. A retry then returned 404, because the
row was already gone.

`delete_files` isolates each blob's deletion so a failure only ever costs
its own file: the response still reports success, every other file is still
attempted, and the failure is logged (with the file name -- the only
remaining handle on an orphaned blob) instead of raised. A missing blob is
untouched by this: `delete_file` already treats that as a no-op, so it never
reaches `delete_files`'s `except` and is never logged as a failure.

These tests fail a specific blob's `delete_blob()` call by monkeypatching
`azure.storage.blob.BlobClient.delete_blob` at the class level, keyed by
blob name -- the same real Azurite-backed boundary every other blob test in
this suite uses, just made to fail for one name and succeed for the rest.
"""
import logging

import azure.storage.blob
import httpx

from app.models.evidence import Evidence
from app.storage.blob_storage import delete_file, get_signed_url

from tests.conftest import build_evidence, make_control


def _fail_deleting_blobs(monkeypatch, *file_names_to_fail: str) -> None:
    """Make `delete_blob()` raise for exactly the given blob names, and
    behave normally (a real call against Azurite) for every other name.

    Patched at the `BlobClient` class level rather than on `delete_file` or
    on whichever name a route module imported, so it exercises the real
    failure boundary -- the Azure SDK call itself -- regardless of which
    module ends up calling it.
    """
    original_delete_blob = azure.storage.blob.BlobClient.delete_blob
    failing = set(file_names_to_fail)

    def fake_delete_blob(self, *args, **kwargs):
        if self.blob_name in failing:
            raise RuntimeError(f"simulated transient storage failure deleting {self.blob_name!r}")
        return original_delete_blob(self, *args, **kwargs)

    monkeypatch.setattr(azure.storage.blob.BlobClient, "delete_blob", fake_delete_blob)


def test_evidence_delete_still_reports_success_when_a_blob_delete_fails(
    db_session, admin_client, monkeypatch
):
    """The database delete already committed by the time blobs are removed,
    so a storage failure must not turn that into a failed response."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        control_id=control.id,
    )
    _fail_deleting_blobs(monkeypatch, files[0].file_name)

    response = admin_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_evidence_delete_still_removes_the_other_files_when_one_fails(
    db_session, admin_client, monkeypatch
):
    """The heart of the ticket: one file's storage failure must not abandon
    the rest of the loop. With three files and the middle one failing, both
    the one before it and the one after it must still be gone from storage."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        ("third.png", b"third"),
        control_id=control.id,
    )
    first, second, third = files
    _fail_deleting_blobs(monkeypatch, second.file_name)

    response = admin_client.delete(f"/api/evidence/{evidence.id}")
    assert response.status_code == 204

    # The failing file is, as expected, still there -- nothing retried it.
    assert httpx.get(get_signed_url(second.file_name)).status_code == 200
    # But its failure must not have stopped the siblings from being removed.
    assert httpx.get(get_signed_url(first.file_name)).status_code == 404
    assert httpx.get(get_signed_url(third.file_name)).status_code == 404


def test_evidence_delete_logs_the_failed_file_name_at_error_level(
    db_session, admin_client, monkeypatch, caplog
):
    """A failure must leave a durable trace naming the blob -- that name is
    the only remaining handle on what is now an orphaned blob."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        control_id=control.id,
    )
    failing_name = files[0].file_name
    _fail_deleting_blobs(monkeypatch, failing_name)

    with caplog.at_level(logging.ERROR, logger="app.storage.blob_storage"):
        response = admin_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    error_records = [r for r in caplog.records if r.levelno >= logging.ERROR]
    assert len(error_records) == 1
    assert failing_name in error_records[0].getMessage()


def test_evidence_delete_does_not_log_a_missing_blob_as_a_failure(
    db_session, admin_client, caplog
):
    """A blob that's already gone from storage (a prior failed delete,
    manual cleanup, whatever) is treated as already deleted, per
    `delete_file`'s existing `ResourceNotFoundError` handling -- it must not
    be logged as a new failure."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        control_id=control.id,
    )
    # Remove one blob from storage ahead of the delete request, simulating
    # a pre-existing orphan gap rather than a failure that happens now.
    delete_file(files[0].file_name)

    with caplog.at_level(logging.ERROR, logger="app.storage.blob_storage"):
        response = admin_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    assert [r for r in caplog.records if r.levelno >= logging.ERROR] == []
    assert httpx.get(get_signed_url(files[1].file_name)).status_code == 404


def test_evidence_delete_clean_path_is_unchanged(db_session, admin_client, caplog):
    """When every blob deletes cleanly, behaviour is exactly as before this
    change: success response, every blob gone, nothing logged."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        control_id=control.id,
    )

    with caplog.at_level(logging.ERROR, logger="app.storage.blob_storage"):
        response = admin_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    assert [r for r in caplog.records if r.levelno >= logging.ERROR] == []
    for ef in files:
        assert httpx.get(get_signed_url(ef.file_name)).status_code == 404


def test_evidence_file_delete_still_reports_success_when_its_blob_delete_fails(
    db_session, admin_client, monkeypatch
):
    """`delete_evidence_file` is single-file, so it can't abandon siblings,
    but it can still wrongly fail a request whose database delete already
    succeeded -- the same property the multi-file routes need."""
    evidence, files = build_evidence(db_session, ("only.png", b"only screenshot"))
    (only_file,) = files
    _fail_deleting_blobs(monkeypatch, only_file.file_name)

    response = admin_client.delete(f"/api/evidence/files/{only_file.id}")

    assert response.status_code == 204


def test_control_cascade_delete_survives_a_storage_failure_and_still_removes_siblings(
    db_session, admin_client, monkeypatch
):
    """The same per-file isolation must apply to the cascade routes
    (`delete_control`/`delete_framework`/`delete_product`), not just
    `delete_evidence` -- proving the shared `delete_files` helper, not a
    one-off fix, is what all the routes now go through."""
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first"),
        ("second.png", b"second"),
        control_id=control.id,
    )
    first, second = files
    _fail_deleting_blobs(monkeypatch, first.file_name)

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 204
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0
    # The failing blob's sibling must still have been removed, despite the
    # failure earlier in the same cascade's file list.
    assert httpx.get(get_signed_url(second.file_name)).status_code == 404
