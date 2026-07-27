"""
Every stored file reference (`"/uploads/{blob_name}"`) is converted to a
short-lived Azure SAS URL at read/serialization time — never stored that
way — and the old unauthenticated `GET /uploads/{filename}` route is gone
(see ADR 0003 / issue #9).

ADR 0003's security case rests on exactly two properties of that signed
link: it expires quickly, and it is read-only. `_assert_signed` below pins
both, not just that a signature and an expiry are present, so every test in
this file that calls it — across the Evidence routes and the Agent Task
result path — enforces the promise, not just the mechanism.

`conftest.py` sets `AZURE_STORAGE_CONNECTION_STRING` to a well-formed,
parseable account-key connection string (the standard Azurite emulator
credential), so `generate_blob_sas` can sign locally with no network call.
A handful of tests below also perform a real, unauthenticated HTTP request
against the signed link — against Azurite, not the app — because the real
property that matters is whether a fetch of the link works (and a write
against it doesn't), not what its query string says.
"""
from datetime import datetime, timedelta, timezone
from urllib.parse import parse_qs, urlparse

import httpx

from app.models.agent_task import AgentTask
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.storage.blob_storage import get_signed_url

from tests.conftest import upload_blob

# Generous tolerance above ADR 0003's 15-minute lifetime, to absorb wall-clock
# slack in the test run. Deliberately not derived from
# `SIGNED_URL_EXPIRY_MINUTES` itself — the whole point is to fail if that
# constant (or the expiry it produces) is ever materially lengthened, and a
# tolerance computed from the same constant it is meant to check would move
# in lockstep with it and never catch that.
_MAX_ACCEPTABLE_LIFETIME = timedelta(minutes=20)


def _assert_signed(url: str) -> None:
    """A signed URL is not the bare stored reference: it points straight at
    the blob, is good for read only, and expires within the short window
    ADR 0003 commits to."""
    assert not url.startswith("/uploads/"), url
    query = parse_qs(urlparse(url).query)
    assert "sig" in query, url
    assert "se" in query, url

    # Read-only: a write- or delete-capable link would carry more than "r"
    # in Azure's "sp" (signed permissions) parameter.
    assert query.get("sp") == ["r"], f"expected a read-only link, got sp={query.get('sp')!r}: {url}"

    # Expires quickly: pins the *window*, not merely that "se" is present,
    # so a link that lasts a year still fails this assertion.
    expiry = datetime.fromisoformat(query["se"][0].replace("Z", "+00:00"))
    now = datetime.now(timezone.utc)
    assert now < expiry <= now + _MAX_ACCEPTABLE_LIFETIME, (
        f"expected an expiry within {_MAX_ACCEPTABLE_LIFETIME} of now, got "
        f"{expiry} (now={now}): {url}"
    )


def test_evidence_response_file_url_is_signed_not_uploads_path(engineer_client, db_session):
    evidence = Evidence(
        title="Screenshot of control panel",
        file_name="abc123.png",
        file_url="/uploads/abc123.png",
        created_by="engineer@example.com",
    )
    db_session.add(evidence)
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    _assert_signed(response.json()["file_url"])


def test_evidence_file_response_file_url_is_signed(engineer_client, db_session):
    evidence = Evidence(
        title="Screenshot of control panel",
        file_name="abc123.png",
        file_url="/uploads/abc123.png",
        created_by="engineer@example.com",
    )
    db_session.add(evidence)
    db_session.flush()
    db_session.add(
        EvidenceFile(
            evidence_id=evidence.id,
            file_name="def456.png",
            file_url="/uploads/def456.png",
            sort_order=0,
        )
    )
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    files = response.json()["files"]
    assert len(files) == 1
    _assert_signed(files[0]["file_url"])
    # The two files point at different blobs, so their signed URLs must differ.
    assert files[0]["file_url"] != response.json()["file_url"]


def test_evidence_list_signs_every_row(engineer_client, db_session):
    db_session.add(
        Evidence(
            title="One",
            file_name="one.png",
            file_url="/uploads/one.png",
            created_by="engineer@example.com",
        )
    )
    db_session.commit()

    response = engineer_client.get("/api/evidence")

    assert response.status_code == 200
    body = response.json()
    assert len(body) == 1
    _assert_signed(body[0]["file_url"])


def test_agent_task_screenshot_payload_is_signed(engineer_client, db_session):
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="completed",
        result={
            "status": "completed",
            "result": "done",
            "screenshots": [
                {"file_name": "shot1.png", "file_url": "/uploads/shot1.png", "subtask": "step 1"},
                {"file_name": "shot2.png", "file_url": "/uploads/shot2.png", "subtask": "step 2"},
            ],
        },
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    screenshots = response.json()["result"]["screenshots"]
    assert len(screenshots) == 2
    _assert_signed(screenshots[0]["file_url"])
    _assert_signed(screenshots[1]["file_url"])
    assert screenshots[0]["file_url"] != screenshots[1]["file_url"]
    # Non-file_url fields pass through untouched.
    assert screenshots[0]["subtask"] == "step 1"


def test_agent_task_progress_screenshot_payload_is_signed(engineer_client, db_session):
    """Live PROGRESS screenshots (posted by the runner after each subtask,
    streamed over SSE) must be signed exactly like the final result's
    screenshots — otherwise the Agent page's live view can't load them
    while the run is still in flight."""
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="running",
        progress={
            "subtasks": [
                {
                    "description": "step 1",
                    "screenshots": [
                        {"file_name": "p1.png", "file_url": "/uploads/p1.png"},
                    ],
                },
                {
                    "description": "step 2",
                    "screenshots": [
                        {"file_name": "p2.png", "file_url": "/uploads/p2.png"},
                    ],
                },
            ],
            "current_index": 1,
        },
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    subtasks = response.json()["progress"]["subtasks"]
    assert len(subtasks) == 2
    _assert_signed(subtasks[0]["screenshots"][0]["file_url"])
    _assert_signed(subtasks[1]["screenshots"][0]["file_url"])
    assert subtasks[0]["screenshots"][0]["file_url"] != subtasks[1]["screenshots"][0]["file_url"]
    # Non-file_url fields pass through untouched.
    assert subtasks[0]["description"] == "step 1"


def test_agent_task_progress_without_screenshots_is_unaffected(engineer_client, db_session):
    """Defensive cases: no progress at all, and subtasks that carry no
    screenshots (or aren't even dicts) — none of these should blow up the
    signing validator."""
    task = AgentTask(user_email="engineer@example.com", prompt="do something", status="queued")
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    assert response.json()["progress"] is None

    task2 = AgentTask(
        user_email="engineer@example.com",
        prompt="do something else",
        status="running",
        progress={"subtasks": [{"description": "no screenshots yet"}], "current_index": 0},
    )
    db_session.add(task2)
    db_session.commit()

    response2 = engineer_client.get(f"/api/agent/tasks/{task2.id}")

    assert response2.status_code == 200
    assert response2.json()["progress"]["subtasks"][0].get("screenshots") is None


def test_agent_task_list_signs_screenshots_too(engineer_client, db_session):
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="completed",
        result={"status": "completed", "screenshots": [{"file_name": "a.png", "file_url": "/uploads/a.png"}]},
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get("/api/agent/tasks")

    assert response.status_code == 200
    body = response.json()
    assert len(body) == 1
    _assert_signed(body[0]["result"]["screenshots"][0]["file_url"])


def test_agent_task_without_screenshots_is_unaffected(engineer_client, db_session):
    """A task with no result yet (still queued) must not blow up the signing
    validator — `result` is None."""
    task = AgentTask(user_email="engineer@example.com", prompt="do something", status="queued")
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    assert response.json()["result"] is None


def test_uploads_route_no_longer_exists(client):
    """The old unauthenticated proxy route is gone entirely — not just
    auth-gated, actually removed."""
    response = client.get("/uploads/anything.png")

    assert response.status_code == 404


def test_signed_link_is_fetchable_and_write_is_refused(engineer_client, db_session):
    """The two properties ADR 0003's design rests on, observed rather than
    parsed. A plain unauthenticated GET of the signed link returns the real
    file — the premise the whole design depends on, since an `<img>` tag has
    no way to attach an Authorization header. A write against that same
    link, with no other credential involved, is refused."""
    file_name, file_url = upload_blob("real-fetch.png", b"real evidence bytes")
    evidence = Evidence(
        title="Screenshot of control panel",
        file_name=file_name,
        file_url=file_url,
        created_by="engineer@example.com",
    )
    db_session.add(evidence)
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}")
    signed_url = response.json()["file_url"]

    read = httpx.get(signed_url)
    assert read.status_code == 200
    assert read.content == b"real evidence bytes"

    write = httpx.put(signed_url, content=b"overwritten", headers={"x-ms-blob-type": "BlockBlob"})
    assert write.status_code == 403, write.text

    # And the write really didn't happen — the file reads back unchanged.
    reread = httpx.get(signed_url)
    assert reread.content == b"real evidence bytes"


def test_agent_task_screenshot_signed_link_is_fetchable(engineer_client, db_session):
    """The screenshots carried in an Agent Task's result go through the same
    signing call as the Evidence routes — prove it produces a real,
    fetchable link here too, not only for Evidence."""
    file_name, file_url = upload_blob("agent-shot.png", b"agent screenshot bytes")
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="completed",
        result={
            "status": "completed",
            "result": "done",
            "screenshots": [{"file_name": file_name, "file_url": file_url, "subtask": "step 1"}],
        },
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")
    signed_url = response.json()["result"]["screenshots"][0]["file_url"]

    fetched = httpx.get(signed_url)

    assert fetched.status_code == 200
    assert fetched.content == b"agent screenshot bytes"


def test_signed_link_is_rejected_once_expired(db_session):
    """The expiry claimed in `se` is not just a label the test asserts and
    Azure ignores: a link signed to have already expired is refused for
    real when fetched, exactly as a leaked link is meant to become useless
    a few minutes after being handed out."""
    _file_name, file_url = upload_blob("expired.png", b"stale evidence bytes")

    already_expired = get_signed_url(file_url, expiry_minutes=-1)

    response = httpx.get(already_expired)

    assert response.status_code == 403, response.text


def test_signing_an_already_signed_url_is_a_no_op():
    """Idempotent: handing an already-signed URL back into `get_signed_url`
    must not treat the whole URL — signature, expiry and all — as a blob
    name and sign it again, which silently produces a link to a blob that
    does not exist. Not reachable via a live call path today (FastAPI
    validates a response model, and therefore signs, exactly once), but the
    guard is defensive and cheap, and covers every validator that calls
    `get_signed_url`, since they all go through this one function."""
    _file_name, file_url = upload_blob("pass-through.png", b"pass-through bytes")
    signed_once = get_signed_url(file_url)

    signed_twice = get_signed_url(signed_once)

    assert signed_twice == signed_once

    # Prove the link still genuinely works, not just that the string was
    # copied verbatim.
    response = httpx.get(signed_twice)
    assert response.status_code == 200
    assert response.content == b"pass-through bytes"


def test_signing_a_stored_reference_produces_a_working_link(db_session):
    """A stored file reference (the `"/uploads/{blob_name}"` form `save_file`
    returns) still produces a working signed link — verified by actually
    fetching it, not just by inspecting the SAS parameters."""
    _file_name, file_url = upload_blob("stored-reference.png", b"stored reference bytes")

    signed = get_signed_url(file_url)

    response = httpx.get(signed)
    assert response.status_code == 200
    assert response.content == b"stored reference bytes"
