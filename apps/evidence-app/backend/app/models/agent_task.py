from datetime import datetime
from sqlalchemy import String, DateTime, JSON, ForeignKey, func
from sqlalchemy.orm import Mapped, mapped_column
from app.database import Base


class AgentTask(Base):
    __tablename__ = "agent_tasks"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    user_email: Mapped[str] = mapped_column(String(255), nullable=False, index=True)
    prompt: Mapped[str] = mapped_column(String(4000), nullable=False)
    region_hint: Mapped[str | None] = mapped_column(String(500), nullable=True)
    portal_url: Mapped[str | None] = mapped_column(String(2000), nullable=True)
    control_id: Mapped[int | None] = mapped_column(ForeignKey("controls.id", ondelete="SET NULL"), nullable=True)
    title: Mapped[str | None] = mapped_column(String(500), nullable=True)

    # Frozen copies of the Control's identity at the moment this task was
    # created -- plain text, no foreign key, so `ON DELETE SET NULL` on
    # `control_id` above can never reach them. Without these, `control_id`
    # going NULL when its Control is deleted is indistinguishable from a
    # `login` task or a `run` started without picking a Control at all --
    # three different situations collapsing into the same NULL. These two
    # columns are the difference between "this run had no Control" and
    # "this run had a Control that is now gone".
    #
    # Deliberately NOT kept in sync if the Control is later renamed: they are
    # set once, at creation, from whatever the Control was called at the
    # time this task actually ran against it. That is audit-correct -- the
    # task describes what really happened, not what the Control is called
    # today -- but it will look like a bug ("why doesn't this match the
    # Control's current name?") to a future reader who doesn't know it's
    # deliberate. It is.
    control_ref_snapshot: Mapped[str | None] = mapped_column(String(50), nullable=True)
    control_title_snapshot: Mapped[str | None] = mapped_column(String(255), nullable=True)

    # run | login — "login" tasks just open a browser at `prompt` (a URL) for the
    # user to manually authenticate (incl. MFA); no LLM steps are run for them.
    kind: Mapped[str] = mapped_column(String(20), nullable=False, default="run")

    # Per-task agent settings — all nullable; runner falls back to its own defaults if None.
    max_steps: Mapped[int | None] = mapped_column(nullable=True)
    use_vision: Mapped[bool | None] = mapped_column(nullable=True)
    max_actions_per_step: Mapped[int | None] = mapped_column(nullable=True)

    # queued | running | completed | failed | cancelled
    status: Mapped[str] = mapped_column(String(20), nullable=False, default="queued", index=True)
    runner_id: Mapped[str | None] = mapped_column(String(100), nullable=True)

    # Set True by the /resume endpoint when the user clicks "Resume" on a task
    # paused via a {PAUSE} marker; the runner polls it and clears it on resume.
    resume_requested: Mapped[bool] = mapped_column(nullable=False, default=False, server_default="0")

    progress: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    result: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    error: Mapped[str | None] = mapped_column(String(2000), nullable=True)

    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
