"""
Deleting a Control an Agent Task still points at -- directly, or via the
Framework or Product above it -- must now SUCCEED, not fail with 409.

Before the fix, `agent_tasks.control_id` was a foreign key to `controls.id`
with no ON DELETE rule, and `agent_tasks` was the only child of the
Product -> Framework -> Control hierarchy with no SQLAlchemy
`relationship()`/cascade -- so the ORM never cleared it before Postgres saw
the delete, and Postgres refused (RESTRICT). Nothing cleared the reference
once a task finished either, so a Control -- or anything above it -- could
be undeletable because of a task that ran months ago.

The fix makes `agent_tasks.control_id` `ON DELETE SET NULL`: the Agent Task
row survives (status, result, timestamps all intact), it just detaches from
the Control that no longer exists. `ON DELETE CASCADE` was explicitly
rejected -- it would destroy audit history and strand `usage_logs` rows,
which reference the task id as a plain string with no FK of their own; the
tests below prove those rows are untouched too.

The delete routes' `except IntegrityError` / 409 handling is kept as a
safety net for any *other* future blocking reference -- `test_control_creation.py`
and friends don't need to know that, but this file's job is specifically the
agent-task case, so it asserts the 409 path no longer fires for it.

Evidence under the deleted Control is unaffected by any of this: it's still
removed by the ORM's own `cascade="all, delete-orphan"` on
`Control.evidence`, exactly as before -- these tests confirm that continues
to hold now that the same delete also succeeds where it used to 409.
"""
from datetime import datetime, timezone

import httpx

from app.models.agent_task import AgentTask
from app.models.control import Control
from app.models.evidence import Evidence
from app.models.framework import Framework
from app.models.product import Product
from app.models.usage_log import UsageLog
from app.storage.blob_storage import get_signed_url

from tests.conftest import build_evidence, make_control


def _finished_task_against(control_id: int, db_session) -> AgentTask:
    """A completed Agent Task with a real result payload and timestamps, so
    tests can assert those survive the Control it pointed at being deleted --
    not just that the row still exists."""
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="check this control",
        status="completed",
        control_id=control_id,
        result={"status": "completed", "result": "looks correct"},
        started_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
        completed_at=datetime(2026, 1, 1, 0, 5, tzinfo=timezone.utc),
    )
    db_session.add(task)
    db_session.commit()
    db_session.refresh(task)
    return task


def _running_task_against(control_id: int, db_session) -> AgentTask:
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="check this control",
        status="running",
        control_id=control_id,
    )
    db_session.add(task)
    db_session.commit()
    db_session.refresh(task)
    return task


def _usage_log_for(task_id: int, db_session) -> UsageLog:
    """A usage/cost row keyed on the task's id as a plain string -- exactly
    how `runner_result` records one (`app/api/routes/agent.py`), and exactly
    the row `ON DELETE CASCADE` would have left stranded had that (rejected)
    approach been used instead of `SET NULL`."""
    usage = UsageLog(
        run_id=str(task_id),
        model="claude-x",
        provider="anthropic",
        input_tokens=100,
        output_tokens=50,
        total_tokens=150,
        llm_calls=1,
        cost_usd=0.01,
        subtask_count=1,
    )
    db_session.add(usage)
    db_session.commit()
    db_session.refresh(usage)
    return usage


def _ids_for(db_session, control: Control) -> tuple[int, int]:
    """(framework_id, product_id) for the chain above a Control."""
    framework = db_session.get(Framework, control.framework_id)
    return framework.id, framework.product_id


def _signed_urls_for(files) -> list[str]:
    return [get_signed_url(ef.file_name) for ef in files]


def _assert_all_gone(signed_urls: list[str]) -> None:
    for url in signed_urls:
        assert httpx.get(url).status_code == 404


def test_deleting_a_control_with_a_finished_agent_task_now_succeeds(db_session, admin_client):
    control = make_control(db_session)
    evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    task = _finished_task_against(control.id, db_session)
    usage = _usage_log_for(task.id, db_session)
    task_id, status, result, started_at, completed_at = (
        task.id,
        task.status,
        task.result,
        task.started_at,
        task.completed_at,
    )

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 204
    assert db_session.get(Control, control.id) is None

    # The Agent Task row survives, detached rather than blocking or vanishing.
    survived = db_session.get(AgentTask, task_id)
    assert survived is not None
    assert survived.control_id is None
    assert survived.status == status
    assert survived.result == result
    assert survived.started_at == started_at
    assert survived.completed_at == completed_at

    # Its usage/cost row is untouched -- it has no FK to control_id at all.
    still_there = db_session.get(UsageLog, usage.id)
    assert still_there is not None
    assert still_there.run_id == str(task_id)
    assert still_there.cost_usd == 0.01

    # Evidence under the Control is still removed, same as before this fix.
    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0
    _assert_all_gone(signed_urls)


def test_deleting_a_framework_whose_controls_task_is_finished_now_succeeds(db_session, admin_client):
    control = make_control(db_session)
    framework_id, _ = _ids_for(db_session, control)
    evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    task = _finished_task_against(control.id, db_session)
    task_id = task.id

    response = admin_client.delete(f"/api/frameworks/{framework_id}")

    assert response.status_code == 204
    assert db_session.get(Framework, framework_id) is None
    assert db_session.get(Control, control.id) is None

    survived = db_session.get(AgentTask, task_id)
    assert survived is not None
    assert survived.control_id is None
    assert survived.status == "completed"

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0
    _assert_all_gone(signed_urls)


def test_deleting_a_product_whose_controls_task_is_finished_now_succeeds(db_session, admin_client):
    control = make_control(db_session)
    _, product_id = _ids_for(db_session, control)
    evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    task = _finished_task_against(control.id, db_session)
    task_id = task.id

    response = admin_client.delete(f"/api/products/{product_id}")

    assert response.status_code == 204
    assert db_session.get(Product, product_id) is None
    assert db_session.get(Control, control.id) is None

    survived = db_session.get(AgentTask, task_id)
    assert survived is not None
    assert survived.control_id is None
    assert survived.status == "completed"

    assert db_session.query(Evidence).filter(Evidence.id == evidence.id).count() == 0
    _assert_all_gone(signed_urls)


def test_deleting_a_control_with_a_running_task_still_succeeds_and_detaches_it(db_session, admin_client):
    """Pins the CURRENT, deliberately incomplete behaviour: there is no guard
    against deleting a Control whose Agent Task is still `queued`/`running`.
    The delete succeeds and `control_id` goes NULL exactly as it would for a
    finished task -- the known, accepted consequence being that any Evidence
    the still-running task later posts (`Evidence.control_id` is nullable)
    lands attached to no Control. Adding a guard for this was deliberately
    deferred; this test documents today's behaviour, not an endorsement of it."""
    control = make_control(db_session)
    task = _running_task_against(control.id, db_session)
    task_id = task.id

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 204
    assert db_session.get(Control, control.id) is None

    survived = db_session.get(AgentTask, task_id)
    assert survived is not None
    assert survived.status == "running"  # unchanged -- nothing cancels it
    assert survived.control_id is None


def test_deleting_a_control_no_task_references_still_succeeds(db_session, admin_client):
    control = make_control(db_session)

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 204
    assert db_session.get(Control, control.id) is None


def test_deleting_a_framework_with_no_referenced_control_still_succeeds(db_session, admin_client):
    control = make_control(db_session)
    framework_id, _ = _ids_for(db_session, control)

    response = admin_client.delete(f"/api/frameworks/{framework_id}")

    assert response.status_code == 204
    assert db_session.get(Framework, framework_id) is None


def test_deleting_a_product_with_no_referenced_control_still_succeeds(db_session, admin_client):
    control = make_control(db_session)
    _, product_id = _ids_for(db_session, control)

    response = admin_client.delete(f"/api/products/{product_id}")

    assert response.status_code == 204
    assert db_session.get(Product, product_id) is None
