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
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.models.submission import Submission
from app.models.usage_log import UsageLog
from app.schemas.agent_task import TaskCreate, TaskOut, TaskProgress, TaskResult
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


@router.post("/tasks", response_model=TaskOut)
def create_task(
    req: TaskCreate,
    user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    task = AgentTask(
        user_email=user.email,
        prompt=req.prompt,
        region_hint=req.region_hint,
        portal_url=req.portal_url,
        control_id=req.control_id,
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
    task = db.query(AgentTask).filter(AgentTask.id == task_id).first()
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
    task = db.query(AgentTask).filter(
        AgentTask.id == task_id, AgentTask.user_email == user.email
    ).first()
    if not task:
        raise HTTPException(404)

    _last_poll[user.email] = datetime.now(timezone.utc)

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
            title=f"AI Agent: {task.title or task.prompt[:80]}",
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
):
    """Runner uploads a PNG screenshot here before posting the task result."""
    # Uploading means the runner is alive and working — keep it "online".
    _last_poll[user.email] = datetime.now(timezone.utc)
    file_name, file_url = save_file(file)
    return {"file_name": file_name, "file_url": file_url}
