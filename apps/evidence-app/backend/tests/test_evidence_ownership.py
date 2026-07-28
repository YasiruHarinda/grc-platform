"""
Evidence get/update endpoints apply the same owner-or-admin authorization
rule: a non-owner, non-admin Engineer is refused (403); the owning Engineer
and any Admin are allowed. This mirrors Agent Task authorization
(test_agent_task_authorization.py) and is now backed by the same shared-helper
pattern: `_authorize_evidence_access(evidence, user)` in
app/api/routes/evidence.py, also used by app/api/routes/submissions.py.

HTTP-level tests below pin externally observable behaviour (status codes),
identically whether the check is inlined per-endpoint or extracted into the
shared helper. A direct unit test of `_authorize_evidence_access` is included
too, for the one case that cannot be produced through the HTTP API: a `None`
Evidence, which the FK constraint on Submission.evidence_id makes
unreachable in practice (see test_submission_scoping.py for the
missing-Evidence 404 behaviour that case feeds).
"""
import pytest
from fastapi import HTTPException

from app.api.routes.evidence import _authorize_evidence_access
from app.auth import User
from app.models.evidence import Evidence

OWNER = "owner-engineer@example.com"


def _make_evidence(db_session, *, created_by: str) -> Evidence:
    evidence = Evidence(
        title="Screenshot of control panel",
        description="test evidence",
        file_name="abc123.png",
        file_url="/uploads/abc123.png",
        control_id=None,
        created_by=created_by,
    )
    db_session.add(evidence)
    db_session.commit()
    db_session.refresh(evidence)
    return evidence


def test_owner_can_get_their_own_evidence(db_session, engineer_client, engineer_user):
    evidence = _make_evidence(db_session, created_by=engineer_user.email)

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    assert response.json()["id"] == evidence.id


def test_non_owner_engineer_is_refused_get(db_session, engineer_client):
    evidence = _make_evidence(db_session, created_by=OWNER)

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 403


def test_admin_can_get_any_evidence(db_session, admin_client):
    evidence = _make_evidence(db_session, created_by=OWNER)

    response = admin_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    assert response.json()["id"] == evidence.id


def test_get_missing_evidence_is_404(engineer_client):
    response = engineer_client.get("/api/evidence/999999")

    assert response.status_code == 404


def test_owner_can_update_their_own_evidence(db_session, engineer_client, engineer_user):
    evidence = _make_evidence(db_session, created_by=engineer_user.email)

    response = engineer_client.patch(f"/api/evidence/{evidence.id}", json={"description": "updated"})

    assert response.status_code == 200
    assert response.json()["description"] == "updated"


def test_non_owner_engineer_is_refused_update(db_session, engineer_client):
    evidence = _make_evidence(db_session, created_by=OWNER)

    response = engineer_client.patch(f"/api/evidence/{evidence.id}", json={"description": "hijacked"})

    assert response.status_code == 403


def test_admin_can_update_any_evidence(db_session, admin_client):
    evidence = _make_evidence(db_session, created_by=OWNER)

    response = admin_client.patch(f"/api/evidence/{evidence.id}", json={"description": "updated by admin"})

    assert response.status_code == 200
    assert response.json()["description"] == "updated by admin"


def test_update_missing_evidence_is_404(engineer_client):
    response = engineer_client.patch("/api/evidence/999999", json={"description": "n/a"})

    assert response.status_code == 404


# --- Direct unit coverage of the shared helper -----------------------------


def test_authorize_evidence_access_raises_404_for_missing_evidence():
    """The FK constraint on Submission.evidence_id means a Submission can
    never legitimately point at a deleted Evidence row over HTTP, so this
    exercises the helper directly for the case app.api.routes.submissions
    relies on it for."""
    user = User(email="engineer@example.com", role="engineer")

    with pytest.raises(HTTPException) as exc_info:
        _authorize_evidence_access(None, user)

    assert exc_info.value.status_code == 404


def test_authorize_evidence_access_allows_owner():
    evidence = Evidence(id=1, title="t", file_name="f", file_url="u", created_by="engineer@example.com")
    user = User(email="engineer@example.com", role="engineer")

    _authorize_evidence_access(evidence, user)  # must not raise


def test_authorize_evidence_access_allows_admin_for_any_owner():
    evidence = Evidence(id=1, title="t", file_name="f", file_url="u", created_by=OWNER)
    user = User(email="admin@example.com", role="admin")

    _authorize_evidence_access(evidence, user)  # must not raise


def test_authorize_evidence_access_refuses_non_owner_non_admin():
    evidence = Evidence(id=1, title="t", file_name="f", file_url="u", created_by=OWNER)
    user = User(email="engineer@example.com", role="engineer")

    with pytest.raises(HTTPException) as exc_info:
        _authorize_evidence_access(evidence, user)

    assert exc_info.value.status_code == 403
