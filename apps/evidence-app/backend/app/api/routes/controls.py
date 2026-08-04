from fastapi import APIRouter, Depends, HTTPException, Query, Response
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session
from app.auth import User, get_current_user
from app.database import get_db
from app.models.control import Control
from app.models.framework import Framework
from app.rbac import require_admin
from app.schemas.control import ControlCreate, ControlResponse, ControlUpdate
from app.storage.blob_storage import delete_files

router = APIRouter(prefix="/controls", tags=["Controls"])


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
