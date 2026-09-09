from datetime import datetime
from sqlalchemy import String, DateTime
from sqlalchemy.orm import Mapped, mapped_column
from app.database import Base


class UsageReset(Base):
    """One row per press of the Cost & Usage page's Reset button.

    A reset is recorded as an event, not applied as a mutation to a single
    stored value: every press adds a new row, and the most recent
    `effective_at` is the cutoff the usage reports honour. That gives an
    attributable history of who moved the reported figure and when, which a
    compliance tool needs, and it means undoing a reset later needs no
    schema change -- just removing the row.

    No foreign keys, in either direction, and no relationship to UsageLog:
    a reset is a boundary the reports filter by, not something joined
    against."""

    __tablename__ = "usage_resets"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    effective_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, index=True)
    reset_by: Mapped[str] = mapped_column(String(255), nullable=False)
