"""
`runner_progress` and `runner_result` (and every other Agent Task handler
except `stream_task`) are plain `def`, so FastAPI runs them in its
threadpool — a worker thread, not the event loop. `stream_task` stays
`async def`: it genuinely awaits `asyncio.Queue.get()` on the event loop.

`_sse_publish` is the seam between the two: it's called from the threadpool
(by `runner_progress`/`runner_result`) but has to deliver into a queue that
belongs to the loop. `asyncio.Queue` is not thread-safe, so a bare
`put_nowait` from a worker thread would be undefined behaviour. The fix is
`loop.call_soon_threadsafe`, with the loop captured once at startup (see
`app/main.py`'s lifespan hook and `app.api.routes.agent.set_event_loop`).

The test below is the behavioural proof: it opens a real SSE stream, then
posts a progress update and a result *over HTTP* — landing on threadpool
worker threads exactly as they would in production — and asserts both
updates actually arrive on the open stream. A plain `TestClient(app)` isn't
enough here: `get_db`/`get_current_user` overrides are set directly (rather
than via the `engineer_client` fixture) because reaching the loop-capture
hook requires the client be used as a context manager
(`with TestClient(app) as client:`), which is what makes FastAPI actually
run the startup/lifespan event — see startup docs in `app/main.py`.

The second test below pins the structural half of the same contract: every
handler that only does blocking work must be a plain `def`, and `stream_task`
must stay `async def`. That's a redundant but cheap, non-flaky guard against
a future regression sliding a blocking handler back to `async def`.
"""
import inspect
import json
import threading

from fastapi.testclient import TestClient

from app.api.routes import agent as agent_routes
from app.auth import get_current_user
from app.database import get_db
from app.main import app
from app.models.agent_task import AgentTask


def test_progress_and_result_posted_from_threadpool_reach_open_stream(db_session, engineer_user):
    task = AgentTask(
        user_email=engineer_user.email,
        prompt="capture the dashboard",
        status="running",
    )
    db_session.add(task)
    db_session.commit()
    db_session.refresh(task)

    app.dependency_overrides[get_db] = lambda: db_session
    app.dependency_overrides[get_current_user] = lambda: engineer_user
    events: list[dict] = []
    try:
        # Entering as a context manager is what triggers the app's startup
        # (lifespan) hook, which is what captures the event loop that
        # `_sse_publish` needs — see the module docstring above.
        with TestClient(app) as client:

            def read_stream() -> None:
                with client.stream("GET", f"/api/agent/tasks/{task.id}/stream") as response:
                    for line in response.iter_lines():
                        if line.startswith("data: "):
                            events.append(json.loads(line[len("data: "):]))

            reader = threading.Thread(target=read_stream)
            reader.start()
            try:
                # Registration into `_sse_listeners` happens synchronously,
                # early in the `stream_task` handler, before any await — this
                # just gives the reader thread a moment to get there first.
                reader.join(timeout=0.5)
                assert reader.is_alive(), "stream ended before we could publish to it"

                progress_response = client.post(
                    f"/api/agent/tasks/{task.id}/progress",
                    json={
                        "subtasks": [{"name": "open the dashboard"}],
                        "current_index": 1,
                    },
                )
                assert progress_response.status_code == 200

                result_response = client.post(
                    f"/api/agent/tasks/{task.id}/result",
                    json={"status": "completed", "result": "captured the dashboard"},
                )
                assert result_response.status_code == 200
            finally:
                # The result above carries a terminal status, which is what
                # makes the SSE generator return and this thread finish.
                reader.join(timeout=10)
            assert not reader.is_alive(), "stream never received the terminal update"
    finally:
        app.dependency_overrides.pop(get_db, None)
        app.dependency_overrides.pop(get_current_user, None)

    assert len(events) >= 3, events
    assert events[0]["status"] == "running"
    assert events[0]["progress"] is None

    progress_events = [e for e in events if e.get("progress") and e["progress"].get("current_index") == 1]
    assert progress_events, events
    assert progress_events[0]["progress"]["subtasks"] == [{"name": "open the dashboard"}]

    assert events[-1]["status"] == "completed"
    assert events[-1]["result"]["result"] == "captured the dashboard"


def test_blocking_agent_handlers_are_plain_def():
    """Every Agent Task handler that only does blocking SQLAlchemy/blob work
    (never `await`s anything) must be a plain `def` so FastAPI runs it in the
    threadpool. `stream_task` is the one exception: it genuinely awaits
    `asyncio.Queue.get()` and must stay on the event loop."""
    blocking_handlers = [
        agent_routes.create_task,
        agent_routes.list_tasks,
        agent_routes.runner_status,
        agent_routes.runner_heartbeat,
        agent_routes.runner_next_task,
        agent_routes.get_task,
        agent_routes.cancel_task,
        agent_routes.resume_task,
        agent_routes.runner_pause_poll,
        agent_routes.runner_progress,
        agent_routes.runner_result,
        agent_routes.upload_screenshot,
    ]
    for handler in blocking_handlers:
        assert not inspect.iscoroutinefunction(handler), f"{handler.__name__} should be a plain def"

    assert inspect.iscoroutinefunction(agent_routes.stream_task), "stream_task must stay async def"
