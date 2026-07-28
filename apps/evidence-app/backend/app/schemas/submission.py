from datetime import datetime
from pydantic import BaseModel


class SubmissionCreate(BaseModel):
    evidence_id: int
    notes: str | None = None


class SubmissionStatusUpdate(BaseModel):
    status: str


class SubmissionResponse(BaseModel):
    id: int
    evidence_id: int
    submitted_by: str
    submitted_at: datetime
    status: str
    notes: str | None

    model_config = {"from_attributes": True}
