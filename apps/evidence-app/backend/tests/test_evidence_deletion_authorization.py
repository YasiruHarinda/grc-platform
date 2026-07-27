"""
`DELETE /api/evidence/{evidence_id}` and `DELETE /api/evidence/files/{file_id}`
apply the same owner-or-admin authorization rule as the read/update endpoints
(test_evidence_ownership.py): the Evidence's owner may delete their own
Evidence or file, an Admin may delete anyone's, and any other Engineer is
refused with 403. Both routes previously required `require_admin` outright --
so an Engineer who owned a piece of Evidence could not delete it or its
files, even though they could already `GET`/`PATCH` it. The fix reuses the
existing `_authorize_evidence_access` helper (see evidence.py) rather than
inventing a second check.

These run against the real Azurite emulator (see conftest.py's
`blob_container` fixture) via `build_evidence`, matching the rest of the
evidence-deletion test suite (test_evidence_file_deletion.py,
test_cascade_evidence_file_deletion.py).
"""
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile

from tests.conftest import build_evidence

OWNER = "owner-engineer@example.com"


def test_owner_can_delete_their_own_evidence(db_session, engineer_client, engineer_user):
    evidence, _files = build_evidence(
        db_session, ("only.png", b"only screenshot"), created_by=engineer_user.email
    )

    response = engineer_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_owner_can_delete_their_own_evidence_file(db_session, engineer_client, engineer_user):
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        created_by=engineer_user.email,
    )
    first_file, _second_file = files

    response = engineer_client.delete(f"/api/evidence/files/{first_file.id}")

    assert response.status_code == 204
    assert db_session.query(EvidenceFile).filter(EvidenceFile.id == first_file.id).count() == 0


def test_non_owner_engineer_is_refused_delete_evidence(db_session, engineer_client):
    evidence, _files = build_evidence(db_session, ("only.png", b"only screenshot"), created_by=OWNER)

    response = engineer_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 403
    # Refused, not just declined to delete: the row must still exist.
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 1


def test_non_owner_engineer_is_refused_delete_evidence_file(db_session, engineer_client):
    evidence, files = build_evidence(db_session, ("only.png", b"only screenshot"), created_by=OWNER)
    (only_file,) = files

    response = engineer_client.delete(f"/api/evidence/files/{only_file.id}")

    assert response.status_code == 403
    assert db_session.query(EvidenceFile).filter(EvidenceFile.id == only_file.id).count() == 1
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 1


def test_admin_can_delete_any_evidence(db_session, admin_client):
    evidence, _files = build_evidence(db_session, ("only.png", b"only screenshot"), created_by=OWNER)

    response = admin_client.delete(f"/api/evidence/{evidence.id}")

    assert response.status_code == 204
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0


def test_admin_can_delete_any_evidence_file(db_session, admin_client):
    evidence, files = build_evidence(
        db_session,
        ("first.png", b"first screenshot"),
        ("second.png", b"second screenshot"),
        created_by=OWNER,
    )
    first_file, _second_file = files

    response = admin_client.delete(f"/api/evidence/files/{first_file.id}")

    assert response.status_code == 204
    assert db_session.query(EvidenceFile).filter(EvidenceFile.id == first_file.id).count() == 0


def test_delete_missing_evidence_is_404(engineer_client):
    response = engineer_client.delete("/api/evidence/999999")

    assert response.status_code == 404


def test_delete_missing_evidence_file_is_404(engineer_client):
    response = engineer_client.delete("/api/evidence/files/999999")

    assert response.status_code == 404
