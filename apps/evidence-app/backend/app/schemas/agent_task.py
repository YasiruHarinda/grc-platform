from __future__ import annotations
from datetime import datetime
from pydantic import BaseModel, model_validator

from app.storage.blob_storage import get_signed_url


class TaskCreate(BaseModel):
    prompt: str
    region_hint: str | None = None
    portal_url: str | None = None
    control_id: int | None = None
    title: str | None = None
    kind: str = "run"  # "run" | "login" — login tasks treat `prompt` as a URL to open
    max_steps: int | None = None
    use_vision: bool | None = None
    max_actions_per_step: int | None = None


class TaskOut(BaseModel):
    id: int
    user_email: str
    prompt: str
    region_hint: str | None
    portal_url: str | None
    control_id: int | None
    title: str | None
    kind: str
    status: str
    runner_id: str | None
    progress: dict | None
    result: dict | None
    error: str | None
    created_at: datetime
    started_at: datetime | None
    completed_at: datetime | None
    max_steps: int | None = None
    use_vision: bool | None = None
    max_actions_per_step: int | None = None
    resume_requested: bool = False

    model_config = {"from_attributes": True}

    @model_validator(mode="after")
    def _sign_screenshot_urls(self) -> "TaskOut":
        # `result["screenshots"]` (set by the runner via POST .../result)
        # carries the same stored `"/uploads/{blob_name}"` references as
        # Evidence — sign each on the way out, same as ADR 0003 elsewhere.
        screenshots = self.result.get("screenshots") if self.result else None
        if screenshots:
            self.result = {
                **self.result,
                "screenshots": [
                    {**shot, "file_url": get_signed_url(shot["file_url"])}
                    if isinstance(shot, dict) and shot.get("file_url")
                    else shot
                    for shot in screenshots
                ],
            }

        # Live PROGRESS screenshots (posted by the runner via POST
        # .../progress after each subtask, streamed over SSE) carry the same
        # raw stored references and need the same treatment — otherwise the
        # Agent page's live view can't load them while the run is in flight.
        subtasks = self.progress.get("subtasks") if self.progress else None
        if subtasks:
            self.progress = {
                **self.progress,
                "subtasks": [
                    {
                        **subtask,
                        "screenshots": [
                            {**shot, "file_url": get_signed_url(shot["file_url"])}
                            if isinstance(shot, dict) and shot.get("file_url")
                            else shot
                            for shot in subtask["screenshots"]
                        ],
                    }
                    if isinstance(subtask, dict) and subtask.get("screenshots")
                    else subtask
                    for subtask in subtasks
                ],
            }
        return self


class TaskProgress(BaseModel):
    subtasks: list[dict] = []
    current_index: int = 0
    total_usage: dict | None = None
    # Set by the runner when a {PAUSE} step is reached; drives the UI's Resume banner.
    paused: bool = False
    pause_message: str | None = None


class TaskResult(BaseModel):
    status: str  # "completed" | "failed"
    result: str | None = None
    error: str | None = None
    screenshots: list[dict] | None = None
    total_usage: dict | None = None
