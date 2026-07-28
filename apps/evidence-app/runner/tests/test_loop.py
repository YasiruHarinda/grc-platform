"""Unit tests for `wso2_runner.loop` — the polling/dispatch loop.

`wso2_runner.agent` imports `browser_use`, which isn't installed in this test
environment, so a fake `wso2_runner.agent` module is injected into
`sys.modules` before `wso2_runner.loop` is ever imported (`loop.py` does
`from wso2_runner.agent import execute_task, open_login_browser,
reset_browser` at module top). Because that's a `from x import y` binding,
loop.py copies those three names into its OWN namespace at import time —
they are not looked up on `wso2_runner.agent` again afterwards. So the
placeholder functions below only need to exist to satisfy the import; every
test that cares about dispatch behaviour monkeypatches `loop.execute_task` /
`loop.open_login_browser` / `loop.reset_browser` directly instead.

`wso2_runner.oauth` and `wso2_runner.client.CloudClient` are imported
differently: `from wso2_runner import oauth` binds the *module* object (so
`oauth.get_access_token` / `oauth.current_email` are looked up dynamically
and can be patched via their dotted path, same as tests/test_client.py
does), while `from wso2_runner.client import CloudClient` binds the class
into loop's namespace — so a fake client class is installed by
monkeypatching `loop.CloudClient` directly.

`run_forever()` is an infinite `while True` polling loop with no built-in
exit seam. Every test here ends it deliberately: the fake CloudClient's
`get_next_task` raises `_StopLoop` (a `BaseException` subclass, not
`Exception`) once its canned queue is exhausted. run_forever's per-iteration
`except Exception` handlers only catch `Exception`, so `_StopLoop` passes
straight through them, unwinds run_forever (via the `finally: aclose()`),
and is caught by each test at the `asyncio.run(...)` call site. `asyncio.sleep`
is monkeypatched globally to return instantly (recording the requested
duration) so no test incurs the loop's real polling/backoff/heartbeat delays.
"""
import asyncio
import os
import platform
import sys
import types

import httpx
import pytest

# --- Fake wso2_runner.agent, injected before loop.py's top-level import ---
_fake_agent = types.ModuleType("wso2_runner.agent")


async def _unused_execute_task(*args, **kwargs):
    raise AssertionError("execute_task placeholder called — test should have patched loop.execute_task")


async def _unused_open_login_browser(*args, **kwargs):
    raise AssertionError("open_login_browser placeholder called — test should have patched loop.open_login_browser")


async def _unused_reset_browser(*args, **kwargs):
    raise AssertionError("reset_browser placeholder called — test should have patched loop.reset_browser")


_fake_agent.execute_task = _unused_execute_task
_fake_agent.open_login_browser = _unused_open_login_browser
_fake_agent.reset_browser = _unused_reset_browser
sys.modules["wso2_runner.agent"] = _fake_agent

from wso2_runner import loop  # noqa: E402 — must follow the sys.modules injection above


class _StopLoop(BaseException):
    """Test-only sentinel used to unwind run_forever's infinite `while True`.

    Subclasses BaseException (not Exception) so it passes straight through
    run_forever's `except Exception` retry handlers instead of being treated
    as just another transient network error.
    """


def _make_fake_client_class(queue, calls):
    """Build a fake CloudClient class.

    `queue` is a list of canned `get_next_task` results (task dicts, or None
    to mean "queue empty this poll") consumed in order; once empty,
    `_StopLoop` is raised to end run_forever's polling loop. `calls` records
    every call made to the fake, keyed by method name, for assertions. Two
    optional keys in `calls`, set by the caller before the run, tune
    behaviour: `_cancel_heartbeat_call` (heartbeat returns True — cancelled —
    starting at that 1-indexed call number) and `_pause_poll_response` (the
    dict pause_poll returns; defaults to an immediate resume).
    """

    class FakeCloudClient:
        def __init__(self, base_url, asgardeo_org, asgardeo_client_id):
            calls["init"] = {
                "base_url": base_url,
                "asgardeo_org": asgardeo_org,
                "asgardeo_client_id": asgardeo_client_id,
            }

        async def get_next_task(self, runner_id):
            calls.setdefault("get_next_task", []).append(runner_id)
            if not queue:
                raise _StopLoop()
            return queue.pop(0)

        async def heartbeat(self, task_id=None):
            calls.setdefault("heartbeat", []).append(task_id)
            cancel_on = calls.get("_cancel_heartbeat_call")
            return cancel_on is not None and len(calls["heartbeat"]) >= cancel_on

        async def post_progress(self, task_id, progress):
            calls.setdefault("post_progress", []).append((task_id, progress))

        async def post_result(self, task_id, result):
            calls.setdefault("post_result", []).append((task_id, result))

        async def pause_poll(self, task_id):
            calls.setdefault("pause_poll", []).append(task_id)
            return calls.get("_pause_poll_response", {"resume_requested": True, "cancelled": False})

        async def upload_screenshot(self, local_path):
            calls.setdefault("upload_screenshot", []).append(local_path)
            return ("https://files.example/shot.png", "shot.png")

        async def aclose(self):
            calls["aclose"] = True

    return FakeCloudClient


def _make_raising_once_client_class(exc, calls):
    """A fake CloudClient whose `get_next_task` raises `exc` on the first
    call (to exercise one of run_forever's retry-backoff branches) and
    `_StopLoop` on the second, ending the test."""

    class RaisingOnceClient:
        def __init__(self, base_url, asgardeo_org, asgardeo_client_id):
            pass

        async def get_next_task(self, runner_id):
            calls.setdefault("get_next_task", []).append(runner_id)
            if len(calls["get_next_task"]) == 1:
                raise exc
            raise _StopLoop()

        async def aclose(self):
            calls["aclose"] = True

    return RaisingOnceClient


@pytest.fixture(autouse=True)
def sleep_calls(monkeypatch):
    """Bootstrap common to every test: a valid client id (so run_forever
    doesn't SystemExit before reaching the loop), a stubbed Asgardeo login,
    and a fast, call-recording `asyncio.sleep` (patched module-wide, so it
    also covers the heartbeat watchdog's and pause-poll's internal sleeps).
    Returns the list of requested sleep durations, in call order.
    """
    monkeypatch.setattr(loop.settings, "ASGARDEO_CLIENT_ID", "test-client-id")
    monkeypatch.setattr("wso2_runner.oauth.get_access_token", lambda *a, **kw: "test-token")
    monkeypatch.setattr("wso2_runner.oauth.current_email", lambda: "user@example.com")

    calls = []

    async def fake_sleep(seconds):
        calls.append(seconds)

    monkeypatch.setattr(asyncio, "sleep", fake_sleep)
    return calls


def _run_bounded(coro):
    """Drive run_forever until it hits the test's _StopLoop sentinel."""
    try:
        asyncio.run(coro)
    except _StopLoop:
        pass
    else:
        raise AssertionError("run_forever returned without hitting the test's _StopLoop sentinel")


def test_runner_id_combines_hostname_and_pid():
    assert loop._runner_id() == f"{platform.node()}-{os.getpid()}"


def test_missing_asgardeo_client_id_raises_system_exit(monkeypatch):
    """A Runner with no Asgardeo client ID configured must fail fast with a
    clear message rather than proceed to sign in."""
    monkeypatch.setattr(loop.settings, "ASGARDEO_CLIENT_ID", "")

    with pytest.raises(SystemExit) as exc_info:
        asyncio.run(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert exc_info.value.code == 1


def test_dispatches_login_reset_and_run_tasks_by_kind(monkeypatch):
    """A `login` task calls open_login_browser, a `reset` task calls
    reset_browser, and any other task calls execute_task — each dispatch's
    return value is posted back via post_result under its own task id."""
    login_task = {"id": 1, "kind": "login", "prompt": "https://sso.example/login"}
    reset_task = {"id": 2, "kind": "reset", "prompt": ""}
    run_task = {"id": 3, "kind": "run", "prompt": "Do the thing"}
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([login_task, reset_task, run_task], calls))

    login_calls = []

    async def fake_open_login_browser(prompt):
        login_calls.append(prompt)
        return {"status": "completed", "result": "logged in", "error": None, "screenshots": []}

    reset_calls = []

    async def fake_reset_browser():
        reset_calls.append(True)
        return {"status": "completed", "result": "reset", "error": None, "screenshots": []}

    execute_calls = []

    async def fake_execute_task(task, on_subtask_done, on_pause):
        execute_calls.append(task["id"])
        return {"status": "completed", "result": "ran", "error": None, "screenshots": []}

    monkeypatch.setattr(loop, "open_login_browser", fake_open_login_browser)
    monkeypatch.setattr(loop, "reset_browser", fake_reset_browser)
    monkeypatch.setattr(loop, "execute_task", fake_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert login_calls == ["https://sso.example/login"]
    assert reset_calls == [True]
    assert execute_calls == [3]

    posted = dict(calls["post_result"])
    assert posted[1] == {"status": "completed", "result": "logged in", "error": None, "screenshots": []}
    assert posted[2] == {"status": "completed", "result": "reset", "error": None, "screenshots": []}
    assert posted[3] == {"status": "completed", "result": "ran", "error": None, "screenshots": []}


def test_empty_queue_sleeps_for_the_poll_interval(monkeypatch, sleep_calls):
    """When get_next_task returns None (queue empty), the loop waits
    poll_interval seconds before polling again instead of busy-looping."""
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([None], calls))

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=4.25))

    assert 4.25 in sleep_calls
    assert len(calls["get_next_task"]) == 2  # the None poll, then the one that raises _StopLoop


def test_connect_error_retries_after_a_10s_backoff(monkeypatch, sleep_calls):
    """A ConnectError (backend unreachable) is swallowed and retried after a
    fixed 10s backoff rather than crashing the runner."""
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_raising_once_client_class(httpx.ConnectError("refused"), calls))

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert 10 in sleep_calls
    assert len(calls["get_next_task"]) == 2


def test_http_status_error_retries_after_a_5s_backoff(monkeypatch, sleep_calls):
    """A non-2xx HTTP response from the backend is swallowed and retried
    after a 5s backoff rather than crashing the runner."""
    calls = {}
    req = httpx.Request("GET", "https://cloud.example/api/agent/tasks/next")
    resp = httpx.Response(500, request=req)
    exc = httpx.HTTPStatusError("server error", request=req, response=resp)
    monkeypatch.setattr(loop, "CloudClient", _make_raising_once_client_class(exc, calls))

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert 5 in sleep_calls
    assert len(calls["get_next_task"]) == 2


def test_unexpected_error_retries_after_a_5s_backoff(monkeypatch, sleep_calls):
    """Any other unexpected exception from get_next_task is also swallowed
    and retried (after 5s) rather than taking the whole runner down."""
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_raising_once_client_class(RuntimeError("weird"), calls))

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert 5 in sleep_calls
    assert len(calls["get_next_task"]) == 2


def test_execute_task_failure_is_caught_and_posted_as_failed_result(monkeypatch):
    """If execute_task raises, the loop must not crash: it posts a "failed"
    result with the exception message and moves on to the next poll."""
    run_task = {"id": 5, "kind": "run", "prompt": "will blow up"}
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([run_task], calls))

    async def failing_execute_task(task, on_subtask_done, on_pause):
        raise ValueError("boom: something in the task went wrong")

    monkeypatch.setattr(loop, "execute_task", failing_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    (task_id, result), = calls["post_result"]
    assert task_id == 5
    assert result["status"] == "failed"
    assert result["result"] is None
    assert "boom: something in the task went wrong" in result["error"]


def test_heartbeat_reporting_cancelled_stops_the_running_task(monkeypatch):
    """If the backend's heartbeat response says the task was cancelled from
    the UI, the in-flight task is cancelled promptly (rather than left to
    run to completion) and a "cancelled" result is posted."""
    run_task = {"id": 9, "kind": "run", "prompt": "long running thing"}
    # Heartbeat call #1 is the initial one _run_task_supervised sends before
    # starting the task; call #2 is the watchdog's first check — report
    # cancelled there so the test doesn't depend on how many times the
    # watchdog loop happens to spin before observing it.
    calls = {"_cancel_heartbeat_call": 2}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([run_task], calls))

    async def hanging_execute_task(task, on_subtask_done, on_pause):
        # Never completes on its own — only cancellation ends this, so the
        # test proves the watchdog actually cancels the in-flight work
        # rather than just observing a task that finished on its own.
        await asyncio.Event().wait()

    monkeypatch.setattr(loop, "execute_task", hanging_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert len(calls["heartbeat"]) >= 2
    assert calls["post_result"] == [
        (9, {"status": "cancelled", "result": None, "error": None, "screenshots": []})
    ]


def test_run_task_progress_and_pause_callbacks_report_to_the_backend(monkeypatch, tmp_path):
    """Exercises the on_subtask_done / on_pause callbacks _run_task hands to
    execute_task: a screenshot is uploaded and progress posted after it; a
    callback with no screenshots still posts progress once (so the UI isn't
    left stale); and a pause posts paused=True, polls pause_poll until
    resumed, then posts paused=False."""
    run_task = {"id": 7, "kind": "run", "prompt": "click around"}
    calls = {"_pause_poll_response": {"resume_requested": True, "cancelled": False}}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([run_task], calls))

    shot = tmp_path / "shot.png"
    shot.write_bytes(b"fake png bytes")

    async def fake_execute_task(task, on_subtask_done, on_pause):
        states = [{"text": "Click the button", "screenshots": []}]
        usage = {"tokens": 10}
        await on_subtask_done(states, 0, [shot], usage)
        await on_subtask_done(states, 0, [], usage)  # no screenshot this call
        await on_pause(states, 0, usage)
        return {"status": "completed", "result": "done", "error": None, "screenshots": states[0]["screenshots"]}

    monkeypatch.setattr(loop, "execute_task", fake_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    (uploaded_path,) = calls["upload_screenshot"]
    assert uploaded_path == shot

    progress_task_ids = [task_id for task_id, _ in calls["post_progress"]]
    assert progress_task_ids == [7, 7, 7, 7]

    paused_flags = [progress.get("paused") for _, progress in calls["post_progress"]]
    # after-screenshot, no-screenshot, pause-start, pause-end
    assert paused_flags == [None, None, True, False]

    first_progress = calls["post_progress"][0][1]
    assert first_progress["subtasks"][0]["screenshots"][0]["file_url"] == "https://files.example/shot.png"
    assert first_progress["subtasks"][0]["screenshots"][0]["file_name"] == "shot.png"

    assert calls["pause_poll"] == [7]

    (task_id, result), = calls["post_result"]
    assert task_id == 7
    assert result["status"] == "completed"


def test_successful_screenshot_upload_deletes_the_local_file(monkeypatch, tmp_path):
    """Ticket #39: once a captured screenshot has been uploaded successfully,
    its local copy under ~/.wso2-runner/ must be removed so Evidence doesn't
    accumulate on disk. Uses a real temp file so the assertion is against
    actual filesystem state, not a mock call."""
    run_task = {"id": 20, "kind": "run", "prompt": "click around"}
    calls = {}
    monkeypatch.setattr(loop, "CloudClient", _make_fake_client_class([run_task], calls))

    shot = tmp_path / "shot.png"
    shot.write_bytes(b"fake png bytes")
    assert shot.exists()  # sanity check before the run

    async def fake_execute_task(task, on_subtask_done, on_pause):
        states = [{"text": "Click the button", "screenshots": []}]
        await on_subtask_done(states, 0, [shot], {"tokens": 1})
        return {"status": "completed", "result": "done", "error": None, "screenshots": states[0]["screenshots"]}

    monkeypatch.setattr(loop, "execute_task", fake_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert calls["upload_screenshot"] == [shot]
    assert not shot.exists()


def test_failed_screenshot_upload_keeps_the_local_file(monkeypatch, tmp_path):
    """Ticket #39: if upload_screenshot raises (e.g. a transient network
    error), the local file must NOT be deleted — otherwise a failed upload
    would permanently lose the captured Evidence instead of leaving it to
    retry or be recovered manually."""
    run_task = {"id": 21, "kind": "run", "prompt": "click around"}
    calls = {}
    base_client = _make_fake_client_class([run_task], calls)

    class FailingUploadClient(base_client):
        async def upload_screenshot(self, local_path):
            calls.setdefault("upload_screenshot", []).append(local_path)
            raise RuntimeError("upload failed: connection reset")

    monkeypatch.setattr(loop, "CloudClient", FailingUploadClient)

    shot = tmp_path / "shot.png"
    shot.write_bytes(b"fake png bytes")
    assert shot.exists()  # sanity check before the run

    async def fake_execute_task(task, on_subtask_done, on_pause):
        states = [{"text": "Click the button", "screenshots": []}]
        await on_subtask_done(states, 0, [shot], {"tokens": 1})
        return {"status": "completed", "result": "done", "error": None, "screenshots": states[0]["screenshots"]}

    monkeypatch.setattr(loop, "execute_task", fake_execute_task)

    _run_bounded(loop.run_forever(cloud_url="https://cloud.example", poll_interval=0))

    assert calls["upload_screenshot"] == [shot]
    assert shot.exists()
