"""
`runner_result` (`POST /agent/tasks/{id}/result`) used to apply every result
posted for a task, with no record that a result had already been recorded.

Real sequence this guards against: the Runner posts a successful result: the
backend commits Evidence, Evidence Files, a Submission and a usage log; the
HTTP response is then lost (read timeout, dropped connection, pod restart).
The Runner never sees the ACK, so its generic error handler
(`runner/wso2_runner/loop.py` ~lines 83-92) posts a SECOND result for the
same task with `status="failed"` and `screenshots: []`. Before the fix, the
backend applied that second post too: the task flipped to `failed` and
`result`/`error`/`completed_at` were overwritten, even though the Evidence
from the first, successful post was intact and correct.

The fix guards on "a result has already been recorded"
(`task.completed_at is not None`) under a row lock, taken on the same task
lookup that `cancel_task` locks — so a second post is a no-op that returns
the same `{"ok": True}` shape as the first, and a concurrent cancel can't be
overwritten by a stale read either.

Deliberately NOT guarding on "status is terminal": a cancelled task must
still have its `result`, `error` and usage log recorded on its first (and
only) result post, because the underlying LLM calls happened and cost real
money. `test_cancelled_task_still_records_result_and_usage_log` below pins
that this still happens after the fix.

These tests assert only on what an HTTP caller can observe — response
bodies and row counts — never on how the handler achieves it (no inspection
of the query/lock).
"""
from app.models.agent_task import AgentTask
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.models.submission import Submission
from app.models.usage_log import UsageLog
from app.schemas.agent_task import TaskResult


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


def _counts(db_session) -> tuple[int, int, int, int]:
    return (
        db_session.query(Evidence).count(),
        db_session.query(EvidenceFile).count(),
        db_session.query(Submission).count(),
        db_session.query(UsageLog).count(),
    )


SUCCESSFUL_RESULT = {
    "status": "completed",
    "result": "captured the dashboard",
    "screenshots": [
        {"file_name": "shot-1.png", "file_url": "/uploads/shot-1.png", "subtask": "open dashboard"},
    ],
    "total_usage": {
        "model": "claude-x",
        "provider": "anthropic",
        "input_tokens": 100,
        "output_tokens": 50,
        "total_tokens": 150,
        "llm_calls": 1,
        "cost_usd": 0.01,
        "subtask_count": 1,
    },
}

# What the Runner's generic error handler posts as a *second* result when it
# never saw the ACK for the first — see loop.py ~83-92: status flips to
# "failed", no screenshots.
CONTRADICTING_SECOND_RESULT = {
    "status": "failed",
    "result": None,
    "error": "connection lost",
    "screenshots": [],
}


def test_replayed_result_does_not_change_counts_or_task_state(db_session, engineer_client, engineer_user):
    task = _make_task(db_session, owner_email=engineer_user.email)

    first_response = engineer_client.post(
        f"/api/agent/tasks/{task.id}/result", json=SUCCESSFUL_RESULT
    )
    assert first_response.status_code == 200

    counts_after_first = _counts(db_session)
    assert counts_after_first == (1, 1, 1, 1), counts_after_first

    db_session.refresh(task)
    status_after_first = task.status
    result_after_first = task.result
    error_after_first = task.error
    completed_at_after_first = task.completed_at
    assert status_after_first == "completed"
    assert completed_at_after_first is not None

    second_response = engineer_client.post(
        f"/api/agent/tasks/{task.id}/result", json=CONTRADICTING_SECOND_RESULT
    )

    # The replay must look, to the caller, exactly like the original success.
    assert second_response.status_code == 200
    assert second_response.json() == first_response.json() == {"ok": True}

    # No new rows of any kind, from the second, contradicting post.
    assert _counts(db_session) == counts_after_first

    db_session.refresh(task)
    assert task.status == status_after_first
    assert task.result == result_after_first
    assert task.error == error_after_first
    assert task.completed_at == completed_at_after_first


def test_cancelled_task_still_records_result_and_usage_log(db_session, engineer_client, engineer_user):
    # Simulates the cancel race's *non*-losing half: the cancel already
    # landed (status == "cancelled") before the runner's one-and-only result
    # post arrives. This must behave exactly as it does today: no Evidence
    # (the cancellation wins over evidence creation), but the result, error
    # and usage log are still recorded because the LLM calls really happened.
    task = _make_task(db_session, owner_email=engineer_user.email, status="cancelled")

    response = engineer_client.post(
        f"/api/agent/tasks/{task.id}/result", json=SUCCESSFUL_RESULT
    )

    assert response.status_code == 200
    assert response.json() == {"ok": True}

    counts = _counts(db_session)
    assert counts == (0, 0, 0, 1), counts  # no Evidence, but the usage log is recorded

    db_session.refresh(task)
    assert task.status == "cancelled"  # cancellation is not overwritten
    # `task.result` is `TaskResult.model_dump()`, so compare against the same
    # shape (it fills in fields SUCCESSFUL_RESULT left implicit, e.g. "error": None).
    assert task.result == TaskResult(**SUCCESSFUL_RESULT).model_dump()
    assert task.completed_at is not None

    usage = db_session.query(UsageLog).one()
    assert usage.run_id == str(task.id)
    assert usage.llm_calls == 1
    assert usage.cost_usd == 0.01
