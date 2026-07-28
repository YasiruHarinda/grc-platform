"""
Atomicity coverage for `DELETE /api/evidence/files/{file_id}`.

Before this fix, deleting an Evidence File spanned two commits: one that
deleted the blob and the EvidenceFile row, and a second, separate commit that
either repointed the primary reference at the surviving file or deleted the
now-empty parent Evidence. A commit is not guaranteed to succeed -- a dropped
connection, a timeout, anything ordinary -- and if the *second* commit failed,
the database was left inconsistent: an Evidence whose primary reference
pointed at a blob that had already been deleted (the repoint case), or an
Evidence with zero files (the empty-parent case). Blobs were also deleted
*before* the first commit, so even a first-commit failure stranded storage:
see `test_deleting_a_control_still_referenced_by_a_task_returns_409` and
friends in test_delete_referenced_parents.py for the same "reject cleanly,
change nothing" property proven for the cascade-delete routes.

The fix does every mutation in one transaction with a single commit, and only
deletes blobs after that commit succeeds. Each test below fails the route's
one remaining commit (via a monkeypatched `Session.commit`, armed to raise on
its very next call) and proves the whole operation rolls back cleanly: no
EvidenceFile deleted, no Evidence mutated or deleted, and -- because blob
deletion now happens strictly after the commit -- no blob removed from
storage either.

This is the same fault injection that exposes the original bug: run against
the pre-fix, two-commit version of the route, "fail the next commit" catches
the route's *first* commit -- the one that already deletes the blob before
committing -- so the "blob still exists" assertions below fail. That is the
revert-proof for this fix: it is not just "one commit instead of two" that
matters, it's deleting blobs only after that commit succeeds.
"""
import httpx
import pytest

from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.storage.blob_storage import get_signed_url

from tests.conftest import build_evidence


def _fail_next_commit(monkeypatch, db_session) -> None:
    """Arm `db_session.commit()` to raise on its very next call, then behave
    normally again. Installed right before firing the request under test, so
    it targets exactly the request's own commit -- not any commit made
    during test setup (`build_evidence` already committed by the time this
    is called)."""
    original_commit = db_session.commit
    state = {"armed": True}

    def fake_commit():
        if state["armed"]:
            state["armed"] = False
            raise RuntimeError("simulated commit failure")
        return original_commit()

    monkeypatch.setattr(db_session, "commit", fake_commit)


def test_commit_failure_during_primary_repoint_leaves_everything_unchanged(
    db_session, admin_client, monkeypatch
):
    """Deleting the primary file while a survivor remains: a failed commit
    must leave the EvidenceFile row, the Evidence's primary reference, and
    every blob exactly as they were -- not a repoint that never happened and
    a blob that's already gone."""
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
    )
    primary_file, survivor_file = files
    original_file_name = evidence.file_name
    original_file_url = evidence.file_url

    _fail_next_commit(monkeypatch, db_session)

    with pytest.raises(RuntimeError, match="simulated commit failure"):
        admin_client.delete(f"/api/evidence/files/{primary_file.id}")

    # The failed commit must leave nothing changed: roll back whatever the
    # failed request queued up so the assertions below see real, persisted
    # state rather than unflushed in-memory changes.
    db_session.rollback()

    db_session.refresh(evidence)
    assert evidence.file_name == original_file_name
    assert evidence.file_url == original_file_url
    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 2
    )
    assert db_session.get(EvidenceFile, primary_file.id) is not None

    # Blob deletion only happens after a successful commit, and this commit
    # never succeeded -- so both blobs must still be there.
    assert httpx.get(get_signed_url(primary_file.file_name)).status_code == 200
    assert httpx.get(get_signed_url(survivor_file.file_name)).status_code == 200


def test_commit_failure_during_empty_parent_deletion_leaves_everything_unchanged(
    db_session, admin_client, monkeypatch
):
    """Deleting the last remaining file, which would otherwise delete the
    whole Evidence: a failed commit must leave the Evidence and its one
    EvidenceFile in place, and the blob untouched."""
    evidence, files = build_evidence(db_session, ("only.png", b"only screenshot"))
    (only_file,) = files

    _fail_next_commit(monkeypatch, db_session)

    with pytest.raises(RuntimeError, match="simulated commit failure"):
        admin_client.delete(f"/api/evidence/files/{only_file.id}")

    db_session.rollback()

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 1
    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 1
    )
    assert db_session.get(EvidenceFile, only_file.id) is not None

    assert httpx.get(get_signed_url(only_file.file_name)).status_code == 200


def test_commit_failure_deleting_whole_evidence_leaves_every_blob(
    db_session, admin_client, monkeypatch
):
    """`DELETE /api/evidence/{id}` deletes every blob for the Evidence, then
    the row. Before the fix it deleted the blobs *before* committing, so a
    failed commit stranded storage: the Evidence and its files survived the
    rollback but their blobs were already gone. The blobs must only be
    removed after the commit succeeds -- this fails commit and proves every
    blob is still there."""
    evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("extra.png", b"extra screenshot"),
    )

    _fail_next_commit(monkeypatch, db_session)

    with pytest.raises(RuntimeError, match="simulated commit failure"):
        admin_client.delete(f"/api/evidence/{evidence.id}")

    db_session.rollback()

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 1
    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 2
    )

    # Both the legacy primary blob and every Evidence File blob must survive
    # a commit that never succeeded.
    assert httpx.get(get_signed_url(evidence.file_name)).status_code == 200
    for ef in files:
        assert httpx.get(get_signed_url(ef.file_name)).status_code == 200
