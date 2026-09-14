from pydantic import BaseModel


class ControlCreate(BaseModel):
    framework_id: int
    control_ref: str
    title: str
    description: str | None = None


class ControlUpdate(BaseModel):
    control_ref: str | None = None
    title: str | None = None
    description: str | None = None


class ControlResponse(BaseModel):
    id: int
    framework_id: int
    control_ref: str
    title: str
    description: str | None

    model_config = {"from_attributes": True}


class ControlBulkRow(BaseModel):
    """One row of a bulk import. Deliberately separate from `ControlCreate`:
    that schema carries a `framework_id` per row, which is exactly the shape
    that would let one bulk request span several Frameworks. A row here
    carries only what varies row to row; the Framework is named once, on
    `ControlBulkCreate` below."""

    control_ref: str
    title: str
    description: str | None = None


class ControlBulkCreate(BaseModel):
    framework_id: int
    controls: list[ControlBulkRow]


class ControlBulkRejection(BaseModel):
    """Why one row of a bulk import wasn't written. `control_ref` here is
    read back from the row itself (not looked up), so a row rejected for
    being blank still names *something* the caller can find in their file."""

    control_ref: str
    reason: str


class ControlBulkResponse(BaseModel):
    """The three outcomes of a bulk import, kept apart rather than merged:
    a bulk row can be created, skipped because it already exists, or
    rejected as unusable, and the dialog on the other end needs to report
    all three without guessing which is which."""

    created: list[ControlResponse]
    skipped: int
    rejected: list[ControlBulkRejection]
