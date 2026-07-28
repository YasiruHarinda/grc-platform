"""
Coverage for the parent-cascade deletes: `DELETE /api/products/{id}`,
`DELETE /api/frameworks/{id}` and `DELETE /api/controls/{id}`.

Each of these walks down to every Evidence beneath the thing being deleted
and removes its blobs from storage. Before this fix, each cascade collected
only the legacy primary `Evidence.file_name` and never walked the Evidence
File list, so every screenshot past the first survived its database row —
unreferenced, unreachable, and billed for indefinitely. `delete_evidence`
(deleting a single Evidence directly) already collected both; these tests
prove the three cascades now match it.

An Evidence with a single file passes even against the unfixed code, because
its primary reference and its only Evidence File point at the same blob.
Every test here builds an Evidence with more than one Evidence File, which
is the case that actually exercises the bug.

These run against the real Azurite emulator (see conftest.py's
`blob_container` fixture), so "the blob is really gone" is observed via a
real HTTP fetch of the signed URL, never inferred from whether some
internal delete function was called.
"""
import httpx

from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.storage.blob_storage import delete_file, get_signed_url

from tests.conftest import build_evidence, make_control, upload_blob


def _assert_all_blobs_gone(file_names: list[str]) -> None:
    for file_name in file_names:
        assert httpx.get(get_signed_url(file_name)).status_code == 404


def _build_evidence_with_distinct_legacy_primary(
    db_session, *files: tuple[str, bytes], control_id: int
) -> tuple[Evidence, list[EvidenceFile], str]:
    """Like `build_evidence`, but the legacy `Evidence.file_name` points at
    its own, separately-uploaded blob rather than the first Evidence File's.

    `build_evidence` always makes the primary and the first Evidence File
    share a blob, so a cascade that walks only the Evidence File list --
    never touching `Evidence.file_name` at all -- still happens to remove
    that blob as a side effect and passes anyway. Seeding a primary blob
    that isn't in the Evidence File list at all is what actually exercises
    the legacy-reference collection each cascade route does alongside its
    Evidence File walk (see e.g. `controls.py`'s `file_names.append(ev.file_name)`).

    Returns (evidence, evidence_files, legacy_primary_blob_name).
    """
    legacy_primary_name, legacy_primary_url = upload_blob(
        "legacy-primary.png", b"legacy primary screenshot, not in the file list"
    )
    uploads = [upload_blob(name, content) for name, content in files]

    evidence = Evidence(
        title="Console screenshot",
        file_name=legacy_primary_name,
        file_url=legacy_primary_url,
        control_id=control_id,
        created_by="engineer@example.com",
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

    return evidence, evidence_files, legacy_primary_name


def test_deleting_a_product_removes_every_evidence_file_beneath_it(db_session, admin_client):
    control = make_control(db_session)
    product_id = control.framework.product_id
    evidence, files, legacy_primary_name = _build_evidence_with_distinct_legacy_primary(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        ("third.png", b"third screenshot"),
        control_id=control.id,
    )
    file_names = [ef.file_name for ef in files]

    response = admin_client.delete(f"/api/products/{product_id}")
    assert response.status_code == 204

    _assert_all_blobs_gone(file_names)
    # The legacy primary is a distinct blob from every Evidence File above --
    # proves the cascade collects it too, not just the Evidence File list.
    _assert_all_blobs_gone([legacy_primary_name])
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_deleting_a_framework_removes_every_evidence_file_beneath_it(db_session, admin_client):
    control = make_control(db_session)
    framework_id = control.framework_id
    evidence, files, legacy_primary_name = _build_evidence_with_distinct_legacy_primary(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        ("third.png", b"third screenshot"),
        control_id=control.id,
    )
    file_names = [ef.file_name for ef in files]

    response = admin_client.delete(f"/api/frameworks/{framework_id}")
    assert response.status_code == 204

    _assert_all_blobs_gone(file_names)
    _assert_all_blobs_gone([legacy_primary_name])
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_deleting_a_control_removes_every_evidence_file_beneath_it(db_session, admin_client):
    control = make_control(db_session)
    evidence, files, legacy_primary_name = _build_evidence_with_distinct_legacy_primary(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        ("third.png", b"third screenshot"),
        control_id=control.id,
    )
    file_names = [ef.file_name for ef in files]

    response = admin_client.delete(f"/api/controls/{control.id}")
    assert response.status_code == 204

    _assert_all_blobs_gone(file_names)
    _assert_all_blobs_gone([legacy_primary_name])
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_cascade_survives_an_evidence_whose_file_was_already_removed_from_storage(
    db_session, admin_client
):
    """A blob can already be gone from storage before the cascade runs (a
    prior failed delete, manual cleanup, whatever the cause) — the cascade
    must not break on that Evidence, and it must still clean up every other
    Evidence beneath the same parent."""
    control = make_control(db_session)
    product_id = control.framework.product_id

    already_gone, _ = build_evidence(
        db_session,
        ("stale-primary.png", b"stale primary"),
        ("stale-secondary.png", b"stale secondary"),
        control_id=control.id,
    )
    still_present, still_present_files = build_evidence(
        db_session,
        ("present-first.png", b"present first"),
        ("present-second.png", b"present second"),
        control_id=control.id,
    )

    # Simulate the blobs for `already_gone` having vanished from storage
    # ahead of the cascade running.
    delete_file(already_gone.file_name)
    for ef in already_gone.files:
        delete_file(ef.file_name)

    response = admin_client.delete(f"/api/products/{product_id}")
    assert response.status_code == 204

    _assert_all_blobs_gone([ef.file_name for ef in still_present_files])
    assert db_session.query(Evidence).filter(Evidence.id == already_gone.id).count() == 0
    assert db_session.query(Evidence).filter(Evidence.id == still_present.id).count() == 0
