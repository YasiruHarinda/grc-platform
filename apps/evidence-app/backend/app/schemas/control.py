from pydantic import BaseModel, Field

# The real SOC 2 export this bulk endpoint exists for is 103 rows. 1000 is
# generous headroom for that case while still bounding the work one request
# can ask for: a request that runs long enough will meet the Choreo
# gateway's own timeout, and a stated, immediate refusal beats a connection
# that dies with the outcome unknown.
#
# It lives here, not beside the route, because the constraint it feeds is on
# the schema below. The route imports it from here; the reverse would be a
# circular import, since this module is imported by that one.
MAX_BULK_CONTROLS = 1000


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
    # Capped here rather than checked inside the route, because a length
    # checked during parsing stops early: pydantic builds one row past the
    # limit and abandons the rest, so a hundred thousand row body never
    # becomes a hundred thousand objects. A check in the handler cannot do
    # that, since the handler is only entered once the whole list is built.
    #
    # The cost is the wording. This answers with pydantic's own validation
    # message rather than a sentence written here, so the dialog on the
    # other end reads the message out of that structure instead.
    controls: list[ControlBulkRow] = Field(max_length=MAX_BULK_CONTROLS)


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
