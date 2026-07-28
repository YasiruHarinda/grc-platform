from datetime import datetime
from typing import Optional
from sqlalchemy import String, ForeignKey, DateTime, func
from sqlalchemy.orm import Mapped, mapped_column, relationship
from app.database import Base


class Evidence(Base):
    __tablename__ = "evidence"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    title: Mapped[str] = mapped_column(String(255), nullable=False)
    description: Mapped[str] = mapped_column(String(1000), nullable=True)
    file_name: Mapped[str] = mapped_column(String(255), nullable=False)
    file_url: Mapped[str] = mapped_column(String(1000), nullable=False)
    control_id: Mapped[Optional[int]] = mapped_column(ForeignKey("controls.id"), nullable=True)
    # Email of the user who uploaded this evidence. Set by get_current_user on
    # write. Pre-Week-1 rows are backfilled to 'legacy@wso2.com' by migration.
    created_by: Mapped[str] = mapped_column(String(255), nullable=False, default="legacy@wso2.com")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), onupdate=func.now())

    control: Mapped["Control"] = relationship("Control", back_populates="evidence")
    submissions: Mapped[list["Submission"]] = relationship(
        "Submission",
        back_populates="evidence",
        cascade="all, delete-orphan",
    )
    files: Mapped[list["EvidenceFile"]] = relationship(
        "EvidenceFile",
        back_populates="evidence",
        cascade="all, delete-orphan",
        order_by="EvidenceFile.sort_order",
    )
