"""
Agent Task get/stream/cancel/resume endpoints all apply the same
owner-or-admin authorization rule: a non-owner, non-admin Engineer is
refused (403); the owning Engineer and any Admin are allowed.

This is intentionally written against the HTTP endpoints (not the
`_authorize_task_access` helper directly), so it pins externally observable
behaviour identically whether the check is duplicated per-endpoint or
extracted into one shared helper (see issue #6 "Cleanups").
"""
from app.models.agent_task import AgentTask


def _make_task(db_session, *, owner_email: str, status: str = "running") -> AgentTask:
    task = AgentTask(
        user_email=owner_email,
        prompt="capture the dashboard",
        status=status,
    )
    db_session.add(task)
    db_session.commit()
    db_session.refresh(task)
    return task


OWNER = "owner-engineer@example.com"


def test_owner_can_get_their_own_task(db_session, engineer_client, engineer_user):
    task = _make_task(db_session, owner_email=engineer_user.email)

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    assert response.json()["id"] == task.id


def test_non_owner_engineer_is_refused_get(db_session, engineer_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 403


def test_admin_can_get_any_task(db_session, admin_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = admin_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    assert response.json()["id"] == task.id


def test_owner_can_cancel_their_own_task(db_session, engineer_client, engineer_user):
    task = _make_task(db_session, owner_email=engineer_user.email)

    response = engineer_client.post(f"/api/agent/tasks/{task.id}/cancel")

    assert response.status_code == 200


def test_non_owner_engineer_is_refused_cancel(db_session, engineer_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = engineer_client.post(f"/api/agent/tasks/{task.id}/cancel")

    assert response.status_code == 403


def test_admin_can_cancel_any_task(db_session, admin_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = admin_client.post(f"/api/agent/tasks/{task.id}/cancel")

    assert response.status_code == 200


def test_owner_can_resume_their_own_task(db_session, engineer_client, engineer_user):
    task = _make_task(db_session, owner_email=engineer_user.email)

    response = engineer_client.post(f"/api/agent/tasks/{task.id}/resume")

    assert response.status_code == 200


def test_non_owner_engineer_is_refused_resume(db_session, engineer_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = engineer_client.post(f"/api/agent/tasks/{task.id}/resume")

    assert response.status_code == 403


def test_admin_can_resume_any_task(db_session, admin_client):
    task = _make_task(db_session, owner_email=OWNER)

    response = admin_client.post(f"/api/agent/tasks/{task.id}/resume")

    assert response.status_code == 200


def test_owner_can_stream_their_own_task(db_session, engineer_client, engineer_user):
    # Use a finished status so the SSE generator yields the initial payload
    # and returns immediately instead of holding the connection open.
    task = _make_task(db_session, owner_email=engineer_user.email, status="completed")

    with engineer_client.stream("GET", f"/api/agent/tasks/{task.id}/stream") as response:
        assert response.status_code == 200


def test_non_owner_engineer_is_refused_stream(db_session, engineer_client):
    task = _make_task(db_session, owner_email=OWNER, status="completed")

    response = engineer_client.get(f"/api/agent/tasks/{task.id}/stream")

    assert response.status_code == 403


def test_admin_can_stream_any_task(db_session, admin_client):
    task = _make_task(db_session, owner_email=OWNER, status="completed")

    with admin_client.stream("GET", f"/api/agent/tasks/{task.id}/stream") as response:
        assert response.status_code == 200
