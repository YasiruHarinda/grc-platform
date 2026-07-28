"""
Coverage for fork issue #70 (implementing the issue's addendum, NOT its
superseded "## Solution" section): evidence blobs get a readable
`product/framework/control/{title}-{uuid}.{ext}` hierarchical name, assigned
once, at write time (`save_file`'s new `prefix`/`label` params, and
`app/storage/blob_paths.py`) -- nothing is ever staged, moved, copied or
renamed afterwards.

Both upload paths go through the same rule:

- The Engineer's manual `POST /api/evidence` already resolves the Control
  before uploading (`create_evidence`), so it just builds the prefix from
  it directly.
- The Runner's `POST /agent/upload-screenshot` gets no task id at all --
  the protocol deliberately stays exactly as it was (see ADR 0001: the
  Runner is never containerised, so any upload-contract change means
  supporting both shapes indefinitely). It resolves the caller's
  currently-*running* Agent Task instead (`/agent/tasks/next` flips exactly
  one task to `running` per user) and walks that task's Control up to its
  Framework/Product. It falls back to the old flat `{uuid}{ext}` path --
  never fails the upload -- when there's no running task, the task's
  `control_id` is `NULL` (routine after the #76 Control-deletion work
  already on this branch), or the Control has since been deleted.

These tests assert only what an observer of the system can see: the blob
name actually sitting in the Azurite emulator, and the reference stored on
the Evidence/EvidenceFile rows -- never how the name was constructed
internally (no inspection of `blob_paths` beyond the fixed fallback labels
it publishes as constants).
"""
import httpx

from app.models.agent_task import AgentTask
from app.models.control import Control
from app.models.evidence import Evidence
from app.models.framework import Framework
from app.models.product import Product
from app.storage.blob_paths import FALLBACK_TITLE_LABEL
from app.storage.blob_storage import get_signed_url

from tests.conftest import uploaded_blob_names


def _make_named_control(
    db_session, *, product_name: str, framework_name: str, control_title: str
) -> Control:
    """Like `tests.conftest.make_control`, but with caller-chosen names --
    needed here because the whole point of these tests is asserting the
    *sanitised* names show up in the blob path."""
    product = Product(name=product_name)
    db_session.add(product)
    db_session.flush()
    framework = Framework(product_id=product.id, name=framework_name)
    db_session.add(framework)
    db_session.flush()
    control = Control(framework_id=framework.id, control_ref="C-1", title=control_title)
    db_session.add(control)
    db_session.commit()
    db_session.refresh(control)
    return control


def _make_task(
    db_session,
    *,
    owner_email: str,
    control_id: int | None,
    status: str = "running",
    title: str | None = None,
    prompt: str = "capture the dashboard",
) -> AgentTask:
    task = AgentTask(
        user_email=owner_email,
        prompt=prompt,
        title=title,
        status=status,
        control_id=control_id,
    )
    db_session.add(task)
    db_session.commit()
    db_session.refresh(task)
    return task


# --- Manual upload: hierarchical path -----------------------------------


def test_manual_upload_lands_at_its_final_hierarchical_path(db_session, engineer_client):
    control = _make_named_control(
        db_session,
        product_name="Payments Platform",
        framework_name="SOC 2",
        control_title="AC-1 Access Control",
    )

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Quarterly Access Review", "control_id": str(control.id)},
        files={"file": ("screenshot.png", b"hierarchical bytes", "image/png")},
    )

    assert response.status_code == 201
    body = response.json()
    evidence = db_session.query(Evidence).filter(Evidence.id == body["id"]).one()

    expected_prefix = "payments-platform/soc-2/ac-1-access-control/"
    assert evidence.file_name.startswith(expected_prefix), evidence.file_name
    assert "quarterly-access-review-" in evidence.file_name
    assert evidence.file_name.endswith(".png")
    # The stored reference still carries the existing /uploads/ prefix.
    assert evidence.file_url == f"/uploads/{evidence.file_name}"

    # The blob is really there under that name, not just claimed by the row.
    assert evidence.file_name in uploaded_blob_names()


# --- Agent screenshot upload: same shape, no task id in the request -----


def test_agent_screenshot_upload_lands_at_the_same_shape_from_the_running_task(
    db_session, engineer_client, engineer_user
):
    """No task id is sent to /agent/upload-screenshot -- it is resolved
    entirely from the authenticated user's currently-running Agent Task."""
    control = _make_named_control(
        db_session,
        product_name="Identity Cloud",
        framework_name="ISO 27001",
        control_title="A9 Access Control",
    )
    _make_task(
        db_session,
        owner_email=engineer_user.email,
        control_id=control.id,
        title="Verify MFA is enforced",
    )

    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"agent screenshot bytes", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()

    expected_prefix = "identity-cloud/iso-27001/a9-access-control/"
    assert body["file_name"].startswith(expected_prefix), body["file_name"]
    # Same title-building rule as runner_result: "AI Agent: {task.title}".
    assert "ai-agent-verify-mfa-is-enforced-" in body["file_name"]
    assert body["file_url"] == f"/uploads/{body['file_name']}"
    assert body["file_name"] in uploaded_blob_names()


def test_agent_screenshot_upload_title_falls_back_to_the_prompt_when_task_has_no_title(
    db_session, engineer_client, engineer_user
):
    """Mirrors `runner_result`'s `task.title or task.prompt[:80]` rule --
    the two title-builders must stay in sync (see `_agent_evidence_title`)."""
    control = _make_named_control(
        db_session,
        product_name="Ops Console",
        framework_name="SOC 2",
        control_title="CC-6 Logical Access",
    )
    _make_task(
        db_session,
        owner_email=engineer_user.email,
        control_id=control.id,
        title=None,
        prompt="Open the IAM console and capture the policy list",
    )

    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"prompt-derived title bytes", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()
    assert "ai-agent-open-the-iam-console-and-capture-the-policy-list-" in body["file_name"]


# --- Agent screenshot upload: fallback to the flat path ------------------


def test_agent_screenshot_upload_with_no_running_task_falls_back_to_flat_path(
    engineer_client, engineer_user
):
    """No Agent Task at all for this user -- the endpoint must still accept
    the upload rather than fail because it couldn't place it in a folder."""
    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"orphan screenshot bytes", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()
    assert "/" not in body["file_name"], body["file_name"]
    assert body["file_url"] == f"/uploads/{body['file_name']}"
    assert body["file_name"] in uploaded_blob_names()


def test_agent_screenshot_upload_with_null_control_id_falls_back_to_flat_path(
    db_session, engineer_client, engineer_user
):
    """A running task with `control_id IS NULL` -- the routine
    post-Control-deletion state after the #76 work already on this branch --
    must not lose the screenshot either."""
    _make_task(db_session, owner_email=engineer_user.email, control_id=None)

    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"post-deletion screenshot bytes", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()
    assert "/" not in body["file_name"], body["file_name"]
    assert body["file_name"] in uploaded_blob_names()


def test_agent_screenshot_upload_ignores_a_non_running_task_of_the_same_user(
    db_session, engineer_client, engineer_user
):
    """A `queued`/`completed`/`cancelled` task of the caller's own must not
    be picked up either -- only a task that is actually `running` right now
    is "this user's currently-running task"."""
    control = _make_named_control(
        db_session,
        product_name="Stale Task Product",
        framework_name="Stale Task Framework",
        control_title="Stale Task Control",
    )
    _make_task(
        db_session, owner_email=engineer_user.email, control_id=control.id, status="completed"
    )

    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"not tied to a finished task", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()
    assert "/" not in body["file_name"], body["file_name"]


def test_agent_screenshot_upload_only_considers_the_callers_own_running_task(
    db_session, engineer_client, engineer_user
):
    """A *different* user's running task must never be picked up for this
    caller -- otherwise one engineer's screenshot could be filed under
    another engineer's in-flight task."""
    control = _make_named_control(
        db_session,
        product_name="Someone Elses Product",
        framework_name="Someone Elses Framework",
        control_title="Someone Elses Control",
    )
    _make_task(db_session, owner_email="someone-else@example.com", control_id=control.id)

    response = engineer_client.post(
        "/api/agent/upload-screenshot",
        files={"file": ("shot.png", b"not someone elses screenshot", "image/png")},
    )

    assert response.status_code == 200
    body = response.json()
    assert "/" not in body["file_name"], body["file_name"]


# --- Sanitisation ----------------------------------------------------------


def test_unsafe_title_characters_are_sanitised_not_turned_into_path_segments(
    db_session, engineer_client
):
    control = _make_named_control(
        db_session,
        product_name="Test Product",
        framework_name="Test Framework",
        control_title="Test Control",
    )
    # A path-traversal attempt, unicode, and a long tail -- all in one title,
    # all user input that must never reach the blob name unsanitised. Kept
    # under Evidence.title's own String(255) column cap (a separate,
    # pre-existing constraint) so this test exercises sanitisation/
    # truncation, not that unrelated column limit.
    dangerous_title = "../../etc/passwd 日本語タイトル " + ("x" * 200)

    response = engineer_client.post(
        "/api/evidence",
        data={"title": dangerous_title, "control_id": str(control.id)},
        files={"file": ("shot.png", b"dangerous title bytes", "image/png")},
    )

    assert response.status_code == 201
    body = response.json()
    evidence = db_session.query(Evidence).filter(Evidence.id == body["id"]).one()

    # Exactly the 3 path segments' worth of "/" -- nothing smuggled in from
    # the title (no "../", no extra directory level).
    assert evidence.file_name.count("/") == 3, evidence.file_name
    prefix, leaf = evidence.file_name.rsplit("/", 1)
    assert prefix == "test-product/test-framework/test-control"
    assert ".." not in leaf
    assert "etc" in leaf  # the safe ASCII portion of the title does survive

    # The label is capped, so even a 300+ char title stays well bounded.
    label_and_uuid = leaf.rsplit(".", 1)[0]
    assert len(label_and_uuid) < 200, label_and_uuid

    assert evidence.file_name in uploaded_blob_names()


def test_title_that_sanitises_to_nothing_still_produces_a_sensible_name(
    db_session, engineer_client
):
    control = _make_named_control(
        db_session,
        product_name="Test Product",
        framework_name="Test Framework",
        control_title="Test Control",
    )

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "!!! 日本語のみ !!!", "control_id": str(control.id)},
        files={"file": ("shot.png", b"blank title bytes", "image/png")},
    )

    assert response.status_code == 201
    body = response.json()
    evidence = db_session.query(Evidence).filter(Evidence.id == body["id"]).one()

    leaf = evidence.file_name.rsplit("/", 1)[1]
    assert leaf.startswith(f"{FALLBACK_TITLE_LABEL}-"), leaf
    assert evidence.file_name in uploaded_blob_names()


# --- Signed URLs and deletion keep working with slashes in the name ------


def test_signed_url_for_a_hierarchical_blob_actually_resolves(db_session, engineer_client):
    control = _make_named_control(
        db_session,
        product_name="Test Product",
        framework_name="Test Framework",
        control_title="Test Control",
    )

    response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console Screenshot", "control_id": str(control.id)},
        files={"file": ("shot.png", b"signed url bytes", "image/png")},
    )
    assert response.status_code == 201
    file_name = response.json()["file_name"]
    assert "/" in file_name  # sanity: this really is a hierarchical name

    fetch = httpx.get(get_signed_url(file_name))
    assert fetch.status_code == 200
    assert fetch.content == b"signed url bytes"


def test_deleting_evidence_with_a_hierarchical_blob_name_still_removes_it(
    db_session, engineer_client, admin_client
):
    control = _make_named_control(
        db_session,
        product_name="Test Product",
        framework_name="Test Framework",
        control_title="Test Control",
    )

    create_response = engineer_client.post(
        "/api/evidence",
        data={"title": "Console Screenshot", "control_id": str(control.id)},
        files={"file": ("shot.png", b"to be deleted", "image/png")},
    )
    assert create_response.status_code == 201
    body = create_response.json()
    file_name = body["file_name"]
    assert "/" in file_name

    delete_response = admin_client.delete(f"/api/evidence/{body['id']}")
    assert delete_response.status_code == 204

    assert file_name not in uploaded_blob_names()
    assert httpx.get(get_signed_url(file_name)).status_code == 404
