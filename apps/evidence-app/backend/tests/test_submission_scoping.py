"""
Submission reads must be scoped to the owning Engineer, mirroring how
Evidence reads are already scoped: a non-admin only sees Submissions tied to
Evidence they created; an Admin sees everything.

Seeds two Engineers' worth of Evidence + Submission rows directly via
`db_session`, then asserts list/get behaviour over HTTP through the
`engineer_client` / `admin_client` fixtures from conftest.py.

get_submission and update_submission_status both delegate their ownership
check to the shared `_authorize_evidence_access` helper (see
app/api/routes/evidence.py and test_evidence_ownership.py), so a missing or
non-owned linked Evidence behaves identically in both: 404 if the Evidence
is gone, 403 if it belongs to someone else. That 404-for-update behaviour is
a deliberate, practically-unreachable-today consequence of the shared helper
owning the "Evidence missing" case rather than get_submission special-casing
it — see the dedupe commit body for the full rationale.
"""
from app.models.evidence import Evidence
from app.models.submission import Submission


def _make_evidence_with_submission(db_session, *, created_by: str, title: str) -> Submission:
    evidence = Evidence(
        title=title,
        description="test evidence",
        file_name=f"{title}.png",
        file_url=f"/uploads/{title}.png",
        control_id=None,
        created_by=created_by,
    )
    db_session.add(evidence)
    db_session.commit()
    db_session.refresh(evidence)

    submission = Submission(
        evidence_id=evidence.id,
        submitted_by=created_by,
        status="pending",
        notes=f"submission for {title}",
    )
    db_session.add(submission)
    db_session.commit()
    db_session.refresh(submission)
    return submission


def test_engineer_lists_only_their_own_submissions(db_session, engineer_client, engineer_user):
    own = _make_evidence_with_submission(db_session, created_by=engineer_user.email, title="own-evidence")
    _make_evidence_with_submission(db_session, created_by="other-engineer@example.com", title="other-evidence")

    response = engineer_client.get("/api/submissions")

    assert response.status_code == 200
    ids = {s["id"] for s in response.json()}
    assert ids == {own.id}


def test_admin_lists_all_submissions(db_session, admin_client, engineer_user):
    mine = _make_evidence_with_submission(db_session, created_by=engineer_user.email, title="own-evidence")
    theirs = _make_evidence_with_submission(db_session, created_by="other-engineer@example.com", title="other-evidence")

    response = admin_client.get("/api/submissions")

    assert response.status_code == 200
    ids = {s["id"] for s in response.json()}
    assert ids == {mine.id, theirs.id}


def test_engineer_reading_own_submission_by_id_succeeds(db_session, engineer_client, engineer_user):
    own = _make_evidence_with_submission(db_session, created_by=engineer_user.email, title="own-evidence")

    response = engineer_client.get(f"/api/submissions/{own.id}")

    assert response.status_code == 200
    assert response.json()["id"] == own.id


def test_engineer_reading_another_engineers_submission_by_id_is_refused(db_session, engineer_client):
    theirs = _make_evidence_with_submission(db_session, created_by="other-engineer@example.com", title="other-evidence")

    response = engineer_client.get(f"/api/submissions/{theirs.id}")

    assert response.status_code == 403


def test_admin_reads_any_submission_by_id(db_session, admin_client):
    theirs = _make_evidence_with_submission(db_session, created_by="some-engineer@example.com", title="some-evidence")

    response = admin_client.get(f"/api/submissions/{theirs.id}")

    assert response.status_code == 200
    assert response.json()["id"] == theirs.id


def test_get_missing_submission_is_404(engineer_client):
    response = engineer_client.get("/api/submissions/999999")

    assert response.status_code == 404


def test_owner_can_update_status_of_their_own_submission(db_session, engineer_client, engineer_user):
    own = _make_evidence_with_submission(db_session, created_by=engineer_user.email, title="own-evidence")

    response = engineer_client.patch(f"/api/submissions/{own.id}", json={"status": "approved"})

    assert response.status_code == 200
    assert response.json()["status"] == "approved"


def test_engineer_cannot_update_status_of_another_engineers_submission(db_session, engineer_client):
    theirs = _make_evidence_with_submission(db_session, created_by="other-engineer@example.com", title="other-evidence")

    response = engineer_client.patch(f"/api/submissions/{theirs.id}", json={"status": "approved"})

    assert response.status_code == 403


def test_admin_can_update_status_of_any_submission(db_session, admin_client):
    theirs = _make_evidence_with_submission(db_session, created_by="some-engineer@example.com", title="some-evidence")

    response = admin_client.patch(f"/api/submissions/{theirs.id}", json={"status": "approved"})

    assert response.status_code == 200
    assert response.json()["status"] == "approved"


def test_update_status_of_missing_submission_is_404(engineer_client):
    response = engineer_client.patch("/api/submissions/999999", json={"status": "approved"})

    assert response.status_code == 404


def _make_evidence(db_session, *, created_by: str, title: str) -> Evidence:
    evidence = Evidence(
        title=title,
        description="test evidence",
        file_name=f"{title}.png",
        file_url=f"/uploads/{title}.png",
        control_id=None,
        created_by=created_by,
    )
    db_session.add(evidence)
    db_session.commit()
    db_session.refresh(evidence)
    return evidence


def test_engineer_creating_submission_against_another_engineers_evidence_is_refused(db_session, engineer_client):
    theirs = _make_evidence(db_session, created_by="other-engineer@example.com", title="other-evidence")

    response = engineer_client.post(
        "/api/submissions", json={"evidence_id": theirs.id, "notes": "sneaky"}
    )

    assert response.status_code == 403


def test_engineer_creating_submission_against_own_evidence_succeeds(db_session, engineer_client, engineer_user):
    own = _make_evidence(db_session, created_by=engineer_user.email, title="own-evidence")

    response = engineer_client.post(
        "/api/submissions", json={"evidence_id": own.id, "notes": "here it is"}
    )

    assert response.status_code == 201
    assert response.json()["evidence_id"] == own.id
    assert response.json()["submitted_by"] == engineer_user.email


def test_admin_creating_submission_against_any_evidence_succeeds(db_session, admin_client):
    theirs = _make_evidence(db_session, created_by="some-engineer@example.com", title="some-evidence")

    response = admin_client.post(
        "/api/submissions", json={"evidence_id": theirs.id, "notes": "admin override"}
    )

    assert response.status_code == 201
    assert response.json()["evidence_id"] == theirs.id


def test_spoofed_submitted_by_in_payload_is_ignored(db_session, engineer_client, engineer_user):
    own = _make_evidence(db_session, created_by=engineer_user.email, title="own-evidence")

    response = engineer_client.post(
        "/api/submissions",
        json={"evidence_id": own.id, "submitted_by": "someone-else@example.com", "notes": "spoof attempt"},
    )

    assert response.status_code == 201
    assert response.json()["submitted_by"] == engineer_user.email


def test_creating_submission_against_missing_evidence_is_404(engineer_client):
    response = engineer_client.post(
        "/api/submissions", json={"evidence_id": 999999, "notes": "no such evidence"}
    )

    assert response.status_code == 404
