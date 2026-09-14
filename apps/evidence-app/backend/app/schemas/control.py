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
    """Why one row of a bulk import wasn't written.

    `row_number` is the row's 1-based position in the list that was sent,
    and it is here because `control_ref` alone cannot always identify the
    row: a row rejected *for having a blank reference* has an empty one,
    which names nothing. The position always points at a line in the file
    the Admin imported, so it is the handle that never goes missing."""

    row_number: int
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
