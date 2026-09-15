from fastapi import APIRouter, Depends, HTTPException, Query, Response
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session
from app.auth import User, get_current_user
from app.database import get_db
from app.models.control import Control
from app.models.framework import Framework
from app.rbac import require_admin
from app.schemas.control import (
    ControlBulkCreate,
    ControlBulkRejection,
    ControlBulkResponse,
    ControlCreate,
    ControlResponse,
    ControlUpdate,
)
from app.storage.blob_storage import delete_files

router = APIRouter(prefix="/controls", tags=["Controls"])

# Read off the columns themselves rather than restated as literals here. A
# migration that widens or narrows one of these would otherwise leave this
# endpoint quietly refusing rows the database would have taken, or waving
# through rows it would refuse -- and the second of those is the one that
# turns a rejected row into a failed import, because a value the database
# refuses arrives as an error on the single INSERT that carries every other
# row with it.
_MAX_CONTROL_REF = Control.__table__.c.control_ref.type.length
_MAX_TITLE = Control.__table__.c.title.type.length
_MAX_DESCRIPTION = Control.__table__.c.description.type.length


@router.get("", response_model=list[ControlResponse])
def list_controls(
    framework_id: int | None = Query(default=None),
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
):
    query = db.query(Control)
    if framework_id:
        query = query.filter(Control.framework_id == framework_id)
    return query.all()


@router.post("", response_model=ControlResponse, status_code=201)
def create_control(payload: ControlCreate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    # Resolve the parent Framework before inserting. Naming a Framework that
    # doesn't exist is a bad request, not a server failure: left to the
    # foreign key it would surface as a raw IntegrityError, i.e. a 500.
    # Matches create_framework's own check on its parent Product.
    if db.query(Framework).filter(Framework.id == payload.framework_id).first() is None:
        raise HTTPException(status_code=404, detail="Framework not found")
    control = Control(**payload.model_dump())
    db.add(control)
    db.commit()
    db.refresh(control)
    return control


@router.post("/bulk", response_model=ControlBulkResponse, status_code=201)
def bulk_create_controls(
    payload: ControlBulkCreate,
    response: Response,
    db: Session = Depends(get_db),
    user: User = Depends(require_admin),
):
    # Nothing checks the row count here. `ControlBulkCreate` caps the list
    # itself (see MAX_BULK_CONTROLS beside it), so an oversized request is
    # refused during parsing, before this function is entered and before
    # the Framework query below costs anything. A second check here would
    # be unreachable.

    # The Framework is named once on the payload, not once per row (see
    # ControlBulkRow), and resolved once here, before any row is looked at.
    # Same not-found message as create_control above, word for word: naming
    # a Framework that doesn't exist is the caller's mistake either way.
    if db.query(Framework).filter(Framework.id == payload.framework_id).first() is None:
        raise HTTPException(status_code=404, detail="Framework not found")

    # Existing references for this Framework, read inside the same
    # transaction the insert below happens in — not from a separate
    # request-earlier query — so the duplicate check is made against the
    # same view of the data the insert will land in. Compared trimmed and
    # lowercased on both sides, matching the browser's own duplicate check,
    # so " c-1" and "C-1 " are the same Control to both.
    existing_refs = {
        control_ref.strip().lower()
        for (control_ref,) in db.query(Control.control_ref).filter(
            Control.framework_id == payload.framework_id
        )
    }

    # Every row is checked before any row is written — this is the point of
    # the endpoint, not a detail of it. Under one transaction a single
    # unusable row would otherwise take the whole import down with it,
    # which would be strictly worse than the row-by-row loop this replaces.
    # A row that fails a check is appended to `rejected`; it never reaches
    # `to_create`, so it can never reach the session at all. Each rejection
    # carries the row's 1-based position as well as its reference, because
    # a row rejected for having a blank reference has no reference to name
    # it by, and the position is then the only handle the Admin has on
    # which line of their file to go and fix.
    seen_refs: set[str] = set()
    to_create: list[Control] = []
    rejected: list[ControlBulkRejection] = []
    skipped = 0

    for position, row in enumerate(payload.controls, start=1):
        # Trimmed the same way update_control trims a field it's given:
        # blank becomes None for description, and reference/title are
        # compared and stored trimmed either way.
        control_ref = row.control_ref.strip()
        title = row.title.strip()
        description = (row.description or "").strip() or None

        if not control_ref:
            rejected.append(ControlBulkRejection(row_number=position, control_ref=control_ref, reason="Reference is blank."))
            continue
        if not title:
            rejected.append(ControlBulkRejection(row_number=position, control_ref=control_ref, reason="Title is blank."))
            continue
        if len(control_ref) > _MAX_CONTROL_REF:
            rejected.append(
                ControlBulkRejection(row_number=position, control_ref=control_ref, reason=f"Reference is longer than {_MAX_CONTROL_REF} characters.")
            )
            continue
        if len(title) > _MAX_TITLE:
            rejected.append(
                ControlBulkRejection(row_number=position, control_ref=control_ref, reason=f"Title is longer than {_MAX_TITLE} characters.")
            )
            continue
        if description is not None and len(description) > _MAX_DESCRIPTION:
            rejected.append(
                ControlBulkRejection(row_number=position, control_ref=control_ref, reason=f"Description is longer than {_MAX_DESCRIPTION} characters.")
            )
            continue

        # A reference already stored under this Framework, and a reference
        # repeated within this same request, land in the same bucket: both
        # are a Control that already exists by the time the insert runs,
        # not a row that's wrong. First occurrence in the request wins; a
        # repeat is silently absorbed here rather than treated as an error.
        key = control_ref.lower()
        if key in existing_refs or key in seen_refs:
            skipped += 1
            continue
        seen_refs.add(key)
        to_create.append(Control(framework_id=payload.framework_id, control_ref=control_ref, title=title, description=description))

    # One transaction, one commit: rows that passed every check above are
    # inserted together, and either all of them land or none do. `flush()`
    # sends the single multi-row INSERT (Postgres returns every assigned id
    # in that same statement) while the transaction is still open, which is
    # what lets the response below read `.id` off each Control without a
    # read-back query per row — the exact per-Control round trip this
    # endpoint exists to remove. Reading it any later would cost that round
    # trip anyway: db.commit() expires every attribute on every object it
    # touched, so a `.id` read after commit reloads from the database one
    # row at a time, silently reintroducing the cost this endpoint exists to
    # remove.
    db.add_all(to_create)
    try:
        db.flush()
        created = [ControlResponse.model_validate(control) for control in to_create]
        db.commit()
    except IntegrityError as exc:
        # Nothing in today's schema should reach this — there's no unique
        # rule on (framework_id, control_ref), so the database itself never
        # refuses a row this loop already decided to write. Kept as a
        # safety net for a constraint added later, in the same spirit as
        # delete_control's own guard further below, so that lands as a
        # clean 409 rather than an unhandled server error.
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail="One or more controls could not be saved due to a conflict.",
        ) from exc

    # 201 only when something was actually created. A request whose every
    # row was already stored, or refused, created no resource, and saying
    # Created would describe the wrong outcome -- a full re-import is an
    # ordinary, successful thing to do and answers 200. The decorator's 201
    # stays as the declared default so the documented response is the usual
    # one; this narrows it for the case that made nothing.
    if not created:
        response.status_code = 200
    return ControlBulkResponse(created=created, skipped=skipped, rejected=rejected)


@router.get("/{control_id}", response_model=ControlResponse)
def get_control(control_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    control = db.query(Control).filter(Control.id == control_id).first()
    if not control:
        raise HTTPException(status_code=404, detail="Control not found")
    return control


@router.patch("/{control_id}", response_model=ControlResponse)
def update_control(control_id: int, payload: ControlUpdate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    control = db.query(Control).filter(Control.id == control_id).first()
    if not control:
        raise HTTPException(status_code=404, detail="Control not found")
    if payload.control_ref is not None:
        control.control_ref = payload.control_ref.strip()
    if payload.title is not None:
        control.title = payload.title.strip()
    if payload.description is not None:
        control.description = payload.description.strip() or None
    db.commit()
    db.refresh(control)
    return control


@router.delete("/{control_id}", status_code=204)
def delete_control(control_id: int, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    control = db.query(Control).filter(Control.id == control_id).first()
    if not control:
        raise HTTPException(status_code=404, detail="Control not found")
    # Mirrors delete_evidence's own collection (the reference here): each
    # Evidence's legacy primary file_name plus every file in its Evidence
    # File list, not the primary alone.
    file_names: list[str] = []
    for ev in control.evidence:
        file_names.extend({ef.file_name for ef in ev.files})
        file_names.append(ev.file_name)
    db.delete(control)
    try:
        db.commit()
    except IntegrityError as exc:
        # Nothing left in this schema should still block a Control delete —
        # agent_tasks.control_id (once the last blocking reference) now
        # detaches with ON DELETE SET NULL rather than refusing the delete.
        # This is kept as a safety net for any future reference Postgres
        # refuses, so it fails as a clean 409 rather than an unhandled
        # server error.
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail="This control is still referenced elsewhere and cannot be deleted.",
        ) from exc
    delete_files(file_names)
    return Response(status_code=204)
