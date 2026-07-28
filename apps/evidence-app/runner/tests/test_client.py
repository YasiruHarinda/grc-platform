"""Unit tests for `wso2_runner.client.CloudClient`.

These pin the runner's side of the contract with the backend task-queue API:
the exact path and method each call uses, that every request carries the
Asgardeo bearer token, that query params and JSON bodies are shaped the way
the backend expects, and that a non-2xx response is surfaced rather than
swallowed. They are the runner's guard that it still matches the merged
backend routes under app/api/routes/agent.py.

No network, no real OAuth, no browser: httpx is driven through a
MockTransport that records the outgoing request and returns a canned
response, and `oauth.get_access_token` is stubbed. Async methods are run via
`asyncio.run`, so no pytest-asyncio plugin is required.
"""
import asyncio
import json
from pathlib import Path

import httpx
import pytest

from wso2_runner.client import CloudClient


@pytest.fixture(autouse=True)
def _stub_token(monkeypatch):
    """Every request goes out with a bearer token; stub the token source so
    the tests never touch the real Asgardeo device flow."""
    monkeypatch.setattr("wso2_runner.oauth.get_access_token", lambda org, cid: "test-token")


def _client_recording_into(requests: list, response_for) -> CloudClient:
    """A CloudClient whose HTTP layer is a MockTransport. Every outgoing
    request is appended to `requests`; `response_for(request)` returns the
    httpx.Response to reply with. Built via __new__ so the real __init__'s
    live AsyncClient is never created."""
    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return response_for(request)

    c = CloudClient.__new__(CloudClient)
    c.base = "https://backend.example"
    c._org = "org"
    c._client_id = "cid"
    c._http = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), timeout=httpx.Timeout(5.0)
    )
    return c


def _run(coro):
    return asyncio.run(coro)


def test_init_strips_trailing_slash():
    """The base URL is normalised so endpoint building never doubles a slash."""
    c = CloudClient("https://backend.example/", "org", "cid")
    assert c.base == "https://backend.example"
    _run(c.aclose())


def test_get_next_task_hits_endpoint_with_runner_id_and_bearer():
    requests: list = []
    task = {"id": 42, "prompt": "do the thing", "status": "running"}
    c = _client_recording_into(requests, lambda req: httpx.Response(200, json=task))

    result = _run(c.get_next_task("runner-1"))
    _run(c.aclose())

    assert result == task
    (req,) = requests
    assert req.method == "GET"
    assert req.url.path == "/api/agent/tasks/next"
    assert req.url.params["runner_id"] == "runner-1"
    assert req.headers["Authorization"] == "Bearer test-token"


def test_get_next_task_returns_none_when_queue_empty():
    """An empty queue is a 200 with a JSON `null` body, which must surface as
    None rather than an empty dict or an error."""
    requests: list = []
    # The backend returns the JSON literal `null` (FastAPI serialising a None
    # response of type `TaskOut | None`), so reproduce that exact body rather
    # than an empty one.
    c = _client_recording_into(
        requests,
        lambda req: httpx.Response(
            200, content=b"null", headers={"content-type": "application/json"}
        ),
    )

    assert _run(c.get_next_task("runner-1")) is None
    _run(c.aclose())


def test_heartbeat_sends_task_id_and_returns_cancelled_flag():
    requests: list = []
    c = _client_recording_into(
        requests, lambda req: httpx.Response(200, json={"ok": True, "cancelled": True})
    )

    cancelled = _run(c.heartbeat(task_id=5))
    _run(c.aclose())

    assert cancelled is True
    (req,) = requests
    assert req.method == "POST"
    assert req.url.path == "/api/agent/heartbeat"
    assert req.url.params["task_id"] == "5"
    assert req.headers["Authorization"] == "Bearer test-token"


def test_post_progress_puts_body_on_the_task_progress_endpoint():
    requests: list = []
    c = _client_recording_into(requests, lambda req: httpx.Response(200, json={"ok": True}))

    progress = {"current_index": 2, "note": "step 2 done"}
    _run(c.post_progress(7, progress))
    _run(c.aclose())

    (req,) = requests
    assert req.method == "POST"
    assert req.url.path == "/api/agent/tasks/7/progress"
    assert json.loads(req.content) == progress


def test_post_result_puts_body_on_the_task_result_endpoint():
    requests: list = []
    c = _client_recording_into(requests, lambda req: httpx.Response(200, json={"ok": True}))

    result = {"status": "completed", "result": "all good", "screenshots": []}
    _run(c.post_result(9, result))
    _run(c.aclose())

    (req,) = requests
    assert req.method == "POST"
    assert req.url.path == "/api/agent/tasks/9/result"
    assert json.loads(req.content) == result


def test_pause_poll_hits_endpoint_and_returns_dict():
    requests: list = []
    c = _client_recording_into(
        requests,
        lambda req: httpx.Response(200, json={"resume_requested": True, "cancelled": False}),
    )

    result = _run(c.pause_poll(11))
    _run(c.aclose())

    assert result == {"resume_requested": True, "cancelled": False}
    (req,) = requests
    assert req.method == "POST"
    assert req.url.path == "/api/agent/tasks/11/pause-poll"
    assert req.headers["Authorization"] == "Bearer test-token"


def test_upload_screenshot_posts_multipart_and_returns_url_and_name(tmp_path: Path):
    requests: list = []
    c = _client_recording_into(
        requests,
        lambda req: httpx.Response(
            200, json={"file_url": "/uploads/abc.png", "file_name": "abc.png"}
        ),
    )
    shot = tmp_path / "shot.png"
    shot.write_bytes(b"\x89PNG\r\n\x1a\n fake png bytes")

    file_url, file_name = _run(c.upload_screenshot(shot))
    _run(c.aclose())

    assert (file_url, file_name) == ("/uploads/abc.png", "abc.png")
    (req,) = requests
    assert req.method == "POST"
    assert req.url.path == "/api/agent/upload-screenshot"
    assert req.headers["content-type"].startswith("multipart/form-data")


def test_a_non_2xx_response_is_raised_not_swallowed():
    """Every call uses raise_for_status, so a backend error must propagate as
    an httpx.HTTPStatusError rather than being returned as data."""
    requests: list = []
    c = _client_recording_into(requests, lambda req: httpx.Response(500, text="boom"))

    with pytest.raises(httpx.HTTPStatusError):
        _run(c.get_next_task("runner-1"))
    _run(c.aclose())
