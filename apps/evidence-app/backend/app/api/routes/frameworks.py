from fastapi import APIRouter, Depends, HTTPException, Query, Response
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session
from app.auth import User, get_current_user
from app.database import get_db
from app.models.framework import Framework
from app.models.product import Product
from app.rbac import require_admin
from app.schemas.framework import FrameworkCreate, FrameworkResponse, FrameworkUpdate
from app.storage.blob_storage import delete_file

router = APIRouter(prefix="/frameworks", tags=["Frameworks"])


@router.get("", response_model=list[FrameworkResponse])
def list_frameworks(
    product_id: int | None = Query(default=None),
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
):
    query = db.query(Framework)
    if product_id is not None:
        query = query.filter(Framework.product_id == product_id)
    return query.order_by(Framework.name).all()


@router.post("", response_model=FrameworkResponse, status_code=201)
def create_framework(payload: FrameworkCreate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    product = db.query(Product).filter(Product.id == payload.product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Product not found")
    framework = Framework(**payload.model_dump())
    db.add(framework)
    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail=f"Framework '{payload.name}' already exists for this product",
        )
    db.refresh(framework)
    return framework


@router.get("/{framework_id}", response_model=FrameworkResponse)
def get_framework(framework_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    framework = db.query(Framework).filter(Framework.id == framework_id).first()
    if not framework:
        raise HTTPException(status_code=404, detail="Framework not found")
    return framework


@router.patch("/{framework_id}", response_model=FrameworkResponse)
def update_framework(framework_id: int, payload: FrameworkUpdate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    framework = db.query(Framework).filter(Framework.id == framework_id).first()
    if not framework:
        raise HTTPException(status_code=404, detail="Framework not found")
    if payload.name is not None:
        framework.name = payload.name.strip()
    if payload.description is not None:
        framework.description = payload.description.strip() or None
    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail="Another framework under this product already uses that name",
        )
    db.refresh(framework)
    return framework


@router.delete("/{framework_id}", status_code=204)
def delete_framework(framework_id: int, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    framework = db.query(Framework).filter(Framework.id == framework_id).first()
    if not framework:
        raise HTTPException(status_code=404, detail="Framework not found")
    # Mirrors delete_evidence's own collection (the reference here): each
    # Evidence's legacy primary file_name plus every file in its Evidence
    # File list, not the primary alone.
    file_names: list[str] = []
    for ctrl in framework.controls:
        for ev in ctrl.evidence:
            file_names.extend({ef.file_name for ef in ev.files})
            file_names.append(ev.file_name)
    db.delete(framework)
    try:
        db.commit()
    except IntegrityError:
        # Deleting a Framework cascades to its Controls, and an Agent Task
        # may still point at one of them (agent_tasks.control_id is a plain FK
        # with no ON DELETE rule, so Postgres refuses). Nothing clears that
        # reference once a task finishes. Turn the refusal into a clean 409
        # rather than letting it surface as an unhandled server error.
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail="A control under this framework is still referenced by one or more agent tasks, so it cannot be deleted.",
        )
    for name in file_names:
        delete_file(name)
    return Response(status_code=204)
