from datetime import datetime
from pydantic import BaseModel, model_validator

from app.storage.blob_storage import get_signed_url


class EvidenceFileOut(BaseModel):
    id: int
    file_name: str
    file_url: str
    subtask: str | None

    model_config = {"from_attributes": True}

    @model_validator(mode="after")
    def _sign_file_url(self) -> "EvidenceFileOut":
        # Convert the stored blob reference to a fresh signed URL on the way
        # out. The DB value is untouched — this only runs at serialization
        # time (see ADR 0003).
        self.file_url = get_signed_url(self.file_url)
        return self


class EvidenceUpdate(BaseModel):
    description: str


class EvidenceResponse(BaseModel):
    id: int
    title: str
    description: str | None
    file_name: str
    file_url: str
    control_id: int | None
    created_by: str
    created_at: datetime
    updated_at: datetime
    files: list[EvidenceFileOut] = []

    model_config = {"from_attributes": True}

    @model_validator(mode="after")
    def _sign_file_url(self) -> "EvidenceResponse":
        self.file_url = get_signed_url(self.file_url)
        return self
