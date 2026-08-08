import asyncio
import json
import uuid
from datetime import datetime, timezone
from pathlib import Path

from fastapi import APIRouter, Depends, HTTPException, Query, UploadFile
from fastapi.responses import StreamingResponse
from sqlalchemy.orm import Session

from app.auth import User, get_current_user
from app.database import get_db
from app.models.agent_task import AgentTask
from app.models.control import Control
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.models.submission import Submission
from app.models.usage_log import UsageLog
from app.schemas.agent_task import TaskCreate, TaskOut, TaskProgress, TaskResult
from app.storage.blob_paths import build_control_prefix, sanitize_title
from app.storage.blob_storage import save_file

router = APIRouter(prefix="/agent", tags=["Agent"])

# Track last runner poll per user_email (in-memory, resets on restart — fine for v1)
_last_poll: dict[str, datetime] = {}

# Per-task SSE pub/sub: task_id → set of asyncio.Queue objects (one per browser tab)
_sse_listeners: dict[int, set] = {}

# The event loop `stream_task`'s asyncio.Queue objects belong to. Captured
# once, from app/main.py's startup hook, while that loop is running — see
# `set_event_loop` below. Every other handler in this module is a plain
# `def`, which FastAPI runs in its threadpool, so `_sse_publish` can be
# called from a worker thread and needs this to hand work back to the loop.
_event_loop: asyncio.AbstractEventLoop | None = None


def set_event_loop(loop: asyncio.AbstractEventLoop) -> None:
    """Record the running event loop so `_sse_publish` can safely reach it
    from other threads. Call once, from a startup/lifespan hook, via
    `asyncio.get_running_loop()` — never from a worker thread."""
    global _event_loop
    _event_loop = loop


def _sse_publish(task_id: int, payload: str) -> None:
    """Push a serialised TaskOut JSON string to every SSE client watching this
    task.

    `runner_progress` and `runner_result` — the only two callers — are plain
    `def` handlers, so FastAPI runs them in its threadpool; this can therefore
    run on a worker thread, not the event loop. asyncio.Queue.put_nowait is
    not thread-safe, so the actual queue writes must happen on the loop
    thread. `loop.call_soon_threadsafe` is itself safe to call from any
    thread — including the loop's own — so this single code path works
    whether the caller happens to be on the loop or on a worker thread.
    """
    loop = _event_loop
    if loop is None:
        # The startup hook hasn't captured the loop yet (e.g. called during
        # import, or before the app has finished starting). Nothing is
        # listening yet either way, so drop rather than crash.
        return

    def _deliver() -> None:
        for q in list(_sse_listeners.get(task_id, set())):
            try:
                q.put_nowait(payload)
            except asyncio.QueueFull:
                pass  # slow client — drop rather than block

    try:
        loop.call_soon_threadsafe(_deliver)
    except RuntimeError:
        pass  # loop already closed (e.g. shutting down) — nothing to do


def _authorize_task_access(task: AgentTask, user: User) -> None:
    """Shared owner-or-admin check for the get/stream/cancel/resume task
    endpoints: only the Agent Task's owner or an Admin may access it."""
    if user.role != "admin" and task.user_email != user.email:
        raise HTTPException(403)


def _agent_evidence_title(task: AgentTask) -> str:
    """The Evidence title an AI-agent run gets, shared by `runner_result`
    (which sets it when the result arrives) and `upload_screenshot` (which
    needs the identical string at upload time, to build the same blob-name
    label -- see fork issue #70's addendum). `task.title` and `task.prompt`
    are both already set at task *creation* (`create_task` above), so both
    call sites can compute this from the same Agent Task row. Kept as one
    helper rather than two copies of the f-string so the two can never drift
    apart.
    """
    return f"AI Agent: {task.title or task.prompt[:80]}"


def _running_task_for(user_email: str, db: Session) -> AgentTask | None:
    """This user's currently-running Agent Task, if any.

    `/agent/tasks/next` (`runner_next_task` below) flips exactly one task to
    `running` for a given user at a time, so this is how `upload_screenshot`
    -- which only ever receives a file and the authenticated user, no task
    id -- resolves which task (and therefore which Control) a screenshot
    belongs to, without adding a `task_id` to the upload contract (which
    would mean the backend supporting both the old and new Runner shapes
    indefinitely; see ADR 0001, the Runner is never containerised and runs
    on individual engineers' laptops).

    Ordered by id descending as a defensive tie-break should more than one
    somehow be `running` at once (not expected in normal operation) --
    picking the most recently claimed task rather than an arbitrary one.
    """
    return (
        db.query(AgentTask)
        .filter(AgentTask.user_email == user_email, AgentTask.status == "running")
        .order_by(AgentTask.id.desc())
        .first()
    )


@router.post("/tasks", response_model=TaskOut)
def create_task(
    req: TaskCreate,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    # Resolve the Control before inserting. Naming a control_id that doesn't
    # exist is a bad request, not a server failure: left to the foreign key
    # it would surface as a raw IntegrityError, i.e. a 500. Matches
    # create_control's own check on its parent Framework
    # (app/api/routes/controls.py).
    #
    # `req.control_id is not None` is the guard that keeps control-less
    # tasks working: `login` tasks never have a Control, and a `run` can be
    # started without picking one either -- both are legitimate and must
    # keep going straight through as NULL. Only a *supplied* id is looked up
    # and validated.
    control = None
    if req.control_id is not None:
        control = db.query(Control).filter(Control.id == req.control_id).first()
        if not control:
            raise HTTPException(status_code=404, detail="Control not found")

    task = AgentTask(
        user_email=user.email,
        prompt=req.prompt,
        region_hint=req.region_hint,
        portal_url=req.portal_url,
        control_id=req.control_id,
        # Snapshot the Control's identity as plain text, frozen at this
        # moment -- see the comment on these columns in app/models/agent_task.py
        # for why they exist and why they're never updated afterwards.
        # `control` is None whenever `req.control_id` was None, so a
        # control-less task correctly gets no snapshot either.
        control_ref_snapshot=control.control_ref if control else None,
        control_title_snapshot=control.title if control else None,
        title=req.title,
        kind=req.kind,
        status="queued",
        max_steps=req.max_steps,
        use_vision=req.use_vision,
        max_actions_per_step=req.max_actions_per_step,
    )
    db.add(task)
    db.commit()
    db.refresh(task)
    return task


@router.get("/tasks", response_model=list[TaskOut])
def list_tasks(
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
    limit: int = 50,
):
    q = db.query(AgentTask)
    if user.role != "admin":
        q = q.filter(AgentTask.user_email == user.email)
    return q.order_by(AgentTask.id.desc()).limit(limit).all()


@router.get("/runner-status")
def runner_status(user: User = Depends(get_current_user)):
    last = _last_poll.get(user.email)
    online = last is not None and (datetime.now(timezone.utc) - last).total_seconds() < 60
    return {"online": online, "last_seen": last.isoformat() if last else None}


@router.post("/heartbeat")
def runner_heartbeat(
    task_id: int | None = None,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """Called by the runner *while it is executing a task*.

    The runner stops polling /tasks/next during execution, so without this the
    UI would wrongly show it as offline the moment a task starts. This keeps the
    "online" heartbeat fresh, and reports back whether the task was cancelled
    from the UI so the runner can stop promptly instead of running to the end.
    """
    _last_poll[user.email] = datetime.now(timezone.utc)
    cancelled = False
    if task_id is not None:
        task = db.query(AgentTask).filter(
            AgentTask.id == task_id, AgentTask.user_email == user.email
        ).first()
        cancelled = bool(task and task.status == "cancelled")
    return {"ok": True, "cancelled": cancelled}


# IMPORTANT: /tasks/next must be defined BEFORE /tasks/{task_id} so FastAPI
# matches the literal path "next" before trying to cast it to int.
@router.get("/tasks/next", response_model=TaskOut | None)
def runner_next_task(
    runner_id: str = Query(...),
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """Runner polls this. Atomically claims one queued task (FOR UPDATE SKIP LOCKED)."""
    _last_poll[user.email] = datetime.now(timezone.utc)
    task = (
        db.query(AgentTask)
        .filter(AgentTask.user_email == user.email, AgentTask.status == "queued")
        .order_by(AgentTask.id.asc())
        .with_for_update(skip_locked=True)
        .first()
    )
    if not task:
        return None
    task.status = "running"
    task.runner_id = runner_id
    task.started_at = datetime.now(timezone.utc)
    db.commit()
    db.refresh(task)
    return task


@router.get("/tasks/{task_id}", response_model=TaskOut)
def get_task(
    task_id: int,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    task = db.query(AgentTask).filter(AgentTask.id == task_id).first()
    if not task:
        raise HTTPException(404)
    _authorize_task_access(task, user)
    return task


@router.get("/tasks/{task_id}/stream")
async def stream_task(
    task_id: int,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """SSE endpoint — pushes TaskOut JSON on every progress/result update.
    Replaces the frontend's 2-second polling loop."""
    task = db.query(AgentTask).filter(AgentTask.id == task_id).first()
    if not task:
        raise HTTPException(404)
    _authorize_task_access(task, user)

    initial_payload = TaskOut.model_validate(task).model_dump_json()
    is_done = task.status in ("completed", "failed", "cancelled")

    q: asyncio.Queue = asyncio.Queue(maxsize=100)
    if not is_done:
        _sse_listeners.setdefault(task_id, set()).add(q)

    async def event_gen():
        try:
            yield f"data: {initial_payload}\n\n"
            if is_done:
                return
            while True:
                try:
                    msg = await asyncio.wait_for(q.get(), timeout=5.0)
                except asyncio.TimeoutError:
                    # Re-read from DB on every timeout so we catch updates written
                    # by a different replica (in-memory _sse_listeners don't cross pods).
                    db.expire_all()
                    refreshed = db.query(AgentTask).filter(AgentTask.id == task_id).first()
                    if refreshed and refreshed.status in ("completed", "failed", "cancelled"):
                        yield f"data: {TaskOut.model_validate(refreshed).model_dump_json()}\n\n"
                        break
                    yield ": ping\n\n"  # keep-alive — nginx resets idle connections otherwise
                    continue
                yield f"data: {msg}\n\n"
                try:
                    if json.loads(msg).get("status") in ("completed", "failed", "cancelled"):
                        break
                except Exception:
                    pass
        finally:
            if task_id in _sse_listeners:
                _sse_listeners[task_id].discard(q)
                if not _sse_listeners[task_id]:
                    del _sse_listeners[task_id]

    return StreamingResponse(
        event_gen(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",   # tells nginx to not buffer this response
            "Connection": "keep-alive",
        },
    )


@router.post("/tasks/{task_id}/cancel")
def cancel_task(
    task_id: int,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    task = db.query(AgentTask).filter(AgentTask.id == task_id).with_for_update().first()
    if not task:
        raise HTTPException(404)
    _authorize_task_access(task, user)
    if task.status in ("completed", "failed", "cancelled"):
        raise HTTPException(400, "Task already finished")
    task.status = "cancelled"
    db.commit()
    return {"ok": True}


@router.post("/tasks/{task_id}/resume")
def resume_task(
    task_id: int,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """Called from the UI when the user clicks "Resume" on a task paused via a
    {PAUSE} marker. Sets a flag the runner polls (see /pause-poll). The runner
    clears the flag once it resumes, so a later {PAUSE} in the same task pauses
    again cleanly."""
    task = db.query(AgentTask).filter(AgentTask.id == task_id).first()
    if not task:
        raise HTTPException(404)
    _authorize_task_access(task, user)
    if task.status in ("completed", "failed", "cancelled"):
        raise HTTPException(400, "Task already finished")
    task.resume_requested = True
    db.commit()
    return {"ok": True}


@router.post("/tasks/{task_id}/pause-poll")
def runner_pause_poll(
    task_id: int,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """Called by the runner *while paused at a {PAUSE} step*. Reports whether the
    user has clicked Resume (and whether the task was cancelled). Reads and
    CLEARS resume_requested atomically so each pause consumes exactly one resume.
    Also keeps the runner's "online" heartbeat fresh while it waits."""
    _last_poll[user.email] = datetime.now(timezone.utc)
    task = db.query(AgentTask).filter(
        AgentTask.id == task_id, AgentTask.user_email == user.email
    ).first()
    if not task:
        raise HTTPException(404)
    resumed = bool(task.resume_requested)
    cancelled = task.status == "cancelled"
    if resumed:
        task.resume_requested = False  # consume it
        db.commit()
    return {"resume_requested": resumed, "cancelled": cancelled}


@router.post("/tasks/{task_id}/progress")
def runner_progress(
    task_id: int,
    payload: TaskProgress,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    task = db.query(AgentTask).filter(
        AgentTask.id == task_id, AgentTask.user_email == user.email
    ).first()
    if not task:
        raise HTTPException(404)
    # Posting progress means the runner is alive and working — keep it "online".
    _last_poll[user.email] = datetime.now(timezone.utc)
    task.progress = payload.model_dump()
    db.commit()
    db.refresh(task)
    _sse_publish(task_id, TaskOut.model_validate(task).model_dump_json())
    return {"ok": True}


@router.post("/tasks/{task_id}/result")
def runner_result(
    task_id: int,
    result: TaskResult,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    task = (
        db.query(AgentTask)
        .filter(AgentTask.id == task_id, AgentTask.user_email == user.email)
        .with_for_update()
        .first()
    )
    if not task:
        raise HTTPException(404)

    # The runner reached us, so it is alive — record that before deciding
    # whether the payload itself is worth applying. A replayed post is still
    # proof of life, and dropping it here would make the runner look offline
    # for no reason.
    _last_poll[user.email] = datetime.now(timezone.utc)

    # A result was already recorded for this task (`completed_at` is set in
    # exactly one place: below). This happens when the Runner posts a result,
    # the response is lost before the Runner sees it (timeout, dropped
    # connection, pod restart), and the Runner's generic error handler posts
    # a second, contradicting result (e.g. status="failed") as it winds down
    # — see runner/wso2_runner/loop.py's exception handler around post_result.
    # The first result already committed Evidence/Evidence Files/Submission
    # and the usage log correctly; a second post must be a no-op that looks,
    # to the caller, exactly like the original success — never overwrite a
    # good result with a stale/contradicting one. The row lock above also
    # makes this check safe against a `cancel_task` racing in between our
    # read and our commit (see that handler's matching lock).
    if task.completed_at is not None:
        return {"ok": True}

    # If the user cancelled from the UI, that decision wins over a normal
    # completion that the runner may post as it winds down.
    cancelled = task.status == "cancelled"
    if not cancelled:
        task.status = result.status
    task.result = result.model_dump()
    task.error = result.error
    task.completed_at = datetime.now(timezone.utc)

    if result.total_usage and result.total_usage.get("llm_calls"):
        usage = result.total_usage
        db.add(UsageLog(
            run_id=str(task.id),
            model=usage.get("model") or "unknown",
            provider=usage.get("provider") or "unknown",
            input_tokens=usage.get("input_tokens", 0),
            output_tokens=usage.get("output_tokens", 0),
            total_tokens=usage.get("total_tokens", 0),
            llm_calls=usage.get("llm_calls", 0),
            cost_usd=usage.get("cost_usd", 0.0),
            subtask_count=usage.get("subtask_count", 1),
        ))

    if result.screenshots and not cancelled:
        first = result.screenshots[0]
        evidence = Evidence(
            title=_agent_evidence_title(task),
            description=first.get("subtask") or task.prompt,
            file_name=first["file_name"],
            file_url=first["file_url"],
            control_id=task.control_id,
            created_by=user.email,
        )
        db.add(evidence)
        db.flush()

        for i, shot in enumerate(result.screenshots):
            db.add(EvidenceFile(
                evidence_id=evidence.id,
                file_name=shot["file_name"],
                file_url=shot["file_url"],
                subtask=shot.get("subtask"),
                sort_order=i,
            ))

        db.add(Submission(
            evidence_id=evidence.id,
            submitted_by="ai-agent",
            status="pending",
            notes=f"Auto-submitted by AI agent ({user.email}). {(result.result or '')[:500]}",
        ))

    db.commit()
    db.refresh(task)
    _sse_publish(task_id, TaskOut.model_validate(task).model_dump_json())
    return {"ok": True}


@router.post("/upload-screenshot")
def upload_screenshot(
    file: UploadFile,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    """Runner uploads a PNG screenshot here before posting the task result.

    This endpoint only ever gets a file and the authenticated user -- no
    task id, and per fork issue #70's addendum, it must stay that way (no
    Runner change, no protocol change). So the hierarchical
    `product/framework/control/` blob-name prefix is derived by resolving
    *this user's currently-running task* (`_running_task_for`) and walking
    its Control up to its Framework and Product.

    Falls back to the old flat `{uuid}{ext}` path -- never fails the
    upload -- whenever that chain can't be resolved: no running task, a
    task with no `control_id` (nullable, and after the #76 work a routine
    post-deletion state, not an error), or a `control_id` whose Control row
    is somehow already gone. A screenshot must never be lost just because we
    couldn't work out where to file it.
    """
    # Uploading means the runner is alive and working — keep it "online".
    _last_poll[user.email] = datetime.now(timezone.utc)

    prefix = ""
    label: str | None = None
    task = _running_task_for(user.email, db)
    if task is not None and task.control_id is not None:
        control = db.query(Control).filter(Control.id == task.control_id).first()
        if control is not None:
            prefix = build_control_prefix(control)
            label = sanitize_title(_agent_evidence_title(task))

    file_name, file_url = save_file(file, prefix=prefix, label=label)
    return {"file_name": file_name, "file_url": file_url}
