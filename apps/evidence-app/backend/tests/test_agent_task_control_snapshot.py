"""
`agent_tasks.control_id` is `ON DELETE SET NULL` (fe1813bddbd8_agent_tasks_
control_id_on_delete_set_null.py): deleting a Control detaches every Agent
Task that targeted it instead of blocking the delete. That keeps Controls
deletable, but it also means `control_id` going NULL has always been able
to mean three different things:

  1. A `login` task, which never had a Control -- it just opens a browser
     for manual MFA.
  2. A `run` task the user started without picking a Control.
  3. A run whose Control existed and was later deleted.

Once `SET NULL` fires, these are indistinguishable -- the Agent Runner page
shows a `Control #42` chip while the Control exists and nothing at all once
it's gone, whether or not it ever had one.

`control_ref_snapshot` / `control_title_snapshot` (`app/models/agent_task.py`)
fix this: plain text, no foreign key, frozen at task creation. The tests
below assert on what an HTTP caller can observe -- response bodies and row
counts -- exactly like `test_delete_referenced_parents.py`, whose Control
lifecycle tests this module builds directly on.

`create_task` also gained a Control lookup it never had before (mirroring
`create_control`'s check on its parent Framework): a `control_id` that
doesn't exist now returns 404 instead of reaching Postgres and failing on
the foreign key with a raw 500. That guard only applies to a *supplied*
id -- an omitted one must keep working exactly as it does today, which the
regression test below pins.
"""
from app.models.agent_task import AgentTask
from app.models.control import Control

from tests.conftest import make_control


def test_create_task_with_control_id_stores_both_snapshots(db_session, engineer_client):
    control = make_control(db_session)

    response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "check this control", "control_id": control.id},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["control_id"] == control.id
    assert body["control_ref_snapshot"] == control.control_ref
    assert body["control_title_snapshot"] == control.title

    stored = db_session.get(AgentTask, body["id"])
    assert stored.control_ref_snapshot == control.control_ref
    assert stored.control_title_snapshot == control.title


def test_deleting_the_control_clears_control_id_but_leaves_both_snapshots_intact(
    db_session, engineer_client, admin_client
):
    """This is the test that states the point of the whole change: the live
    link breaks, exactly as fe1813bddbd8 intends, but the human-readable
    identity of what the run was actually for survives it."""
    control = make_control(db_session)
    create_response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "check this control", "control_id": control.id},
    )
    assert create_response.status_code == 200
    task_id = create_response.json()["id"]

    delete_response = admin_client.delete(f"/api/controls/{control.id}")
    assert delete_response.status_code == 204
    assert db_session.get(Control, control.id) is None

    survived = db_session.get(AgentTask, task_id)
    assert survived.control_id is None
    assert survived.control_ref_snapshot == control.control_ref
    assert survived.control_title_snapshot == control.title

    get_response = engineer_client.get(f"/api/agent/tasks/{task_id}")
    assert get_response.status_code == 200
    body = get_response.json()
    assert body["control_id"] is None
    assert body["control_ref_snapshot"] == control.control_ref
    assert body["control_title_snapshot"] == control.title


def test_run_task_with_no_control_id_stores_no_snapshot(db_session, engineer_client):
    """A `run` task started without picking a Control is legitimate and must
    keep working -- and must not manufacture a snapshot out of nothing."""
    response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "go look around"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["control_id"] is None
    assert body["control_ref_snapshot"] is None
    assert body["control_title_snapshot"] is None


def test_login_task_stores_no_snapshot(db_session, engineer_client):
    """A `login` task only opens a browser at `prompt` (a URL) for manual
    MFA -- it never had a Control, so it must show the same "no snapshot"
    shape as a Control-less run, not be mistaken for a deleted one."""
    response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "https://portal.example.com/login", "kind": "login"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["kind"] == "login"
    assert body["control_id"] is None
    assert body["control_ref_snapshot"] is None
    assert body["control_title_snapshot"] is None


def test_unknown_control_id_returns_404_and_creates_no_task(db_session, engineer_client):
    before_count = db_session.query(AgentTask).count()

    response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "check this control", "control_id": 999999999},
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Control not found"
    assert db_session.query(AgentTask).count() == before_count


def test_request_with_no_control_id_still_succeeds(db_session, engineer_client):
    """Regression guard: omitting `control_id` entirely (as opposed to
    sending it explicitly as null) must still go straight through as NULL,
    not get treated as an id to look up."""
    response = engineer_client.post(
        "/api/agent/tasks",
        json={"prompt": "capture the dashboard"},
    )

    assert response.status_code == 200
    assert response.json()["control_id"] is None
