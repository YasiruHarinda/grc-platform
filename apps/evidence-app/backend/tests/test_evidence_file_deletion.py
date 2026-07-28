"""
Coverage for `DELETE /api/evidence/files/{file_id}` — specifically the case
where the deleted file is the Evidence's *primary* reference
(`Evidence.file_name` / `file_url`) and other Evidence Files remain.

An Evidence records its main screenshot in two places: the primary
file_name/file_url on the Evidence row itself, and the Evidence File list
beside it. Deleting a non-primary Evidence File already worked correctly,
and deleting the last remaining file already cleaned up the whole Evidence.
The gap was deleting the *primary* while others survive: the Evidence went
on advertising a blob that no longer existed, rendering broken even though
good screenshots remained. This file proves the fix — the Evidence is
repointed at a surviving file rather than left dangling or nulled — and
that the two paths which already worked stay working.

These run against the real Azurite emulator (see conftest.py's
`blob_container` fixture), so "the blob is really gone" / "the survivor is
really fetchable" are observed via real HTTP fetches, never inferred from
whether some internal delete function was called. `create_evidence` only
ever accepts one file per request, so a multi-file Evidence is built
directly via the ORM here — the same approach test_signed_urls.py uses.
"""
import httpx

from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.storage.blob_storage import get_signed_url

from tests.conftest import build_evidence as _build_evidence


def test_deleting_the_primary_file_repoints_evidence_at_the_next_survivor(db_session, admin_client):
    """The case this ticket exists for: an Evidence with more than one
    Evidence File, where the deleted one is the primary. A single-file
    Evidence would pass even against the unfixed code, so this needs at
    least two survivor candidates to prove the deterministic ordering
    (lowest remaining sort_order, not whichever row the database happens to
    return first)."""
    evidence, files = _build_evidence(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        ("third.png", b"third screenshot"),
    )
    primary_file, next_file, _last_file = files

    response = admin_client.delete(f"/api/evidence/files/{primary_file.id}")
    assert response.status_code == 204

    db_session.refresh(evidence)

    assert evidence.file_name == next_file.file_name
    assert evidence.file_url == next_file.file_url

    # The old primary's blob is really gone from storage.
    assert httpx.get(get_signed_url(primary_file.file_name)).status_code == 404
    # The new primary reference is really fetchable, not just a string that
    # happens to look right.
    assert httpx.get(get_signed_url(evidence.file_name)).status_code == 200

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 1
    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 2
    )


def test_deleting_a_non_primary_file_leaves_the_primary_reference_untouched(db_session, admin_client):
    evidence, files = _build_evidence(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        ("third.png", b"third screenshot"),
    )
    _primary_file, middle_file, _last_file = files
    original_file_name = evidence.file_name
    original_file_url = evidence.file_url

    response = admin_client.delete(f"/api/evidence/files/{middle_file.id}")
    assert response.status_code == 204

    db_session.refresh(evidence)
    assert evidence.file_name == original_file_name
    assert evidence.file_url == original_file_url

    # The deleted (non-primary) file's own blob is really gone...
    assert httpx.get(get_signed_url(middle_file.file_name)).status_code == 404
    # ...but the primary's blob is untouched.
    assert httpx.get(get_signed_url(evidence.file_name)).status_code == 200

    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 2
    )


def test_deleting_the_last_remaining_file_still_removes_the_evidence_entirely(db_session, admin_client):
    """The path that already worked before this ticket — must keep working
    unchanged by the primary-repointing fix."""
    evidence, files = _build_evidence(db_session, ("only.png", b"only screenshot"))
    (only_file,) = files

    response = admin_client.delete(f"/api/evidence/files/{only_file.id}")
    assert response.status_code == 204

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0
    assert (
        db_session.query(EvidenceFile).filter(EvidenceFile.evidence_id == evidence.id).count()
        == 0
    )
    assert httpx.get(get_signed_url(only_file.file_name)).status_code == 404
