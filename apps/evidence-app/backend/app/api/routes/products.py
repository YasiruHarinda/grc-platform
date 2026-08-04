from fastapi import APIRouter, Depends, HTTPException, Response
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.auth import User, get_current_user
from app.database import get_db
from app.models.product import Product
from app.rbac import require_admin
from app.schemas.product import ProductCreate, ProductResponse, ProductUpdate
from app.storage.blob_storage import delete_files

router = APIRouter(prefix="/products", tags=["Products"])


def _collect_evidence_files(product: Product) -> list[str]:
    """Walk product → frameworks → controls → evidence to gather every blob
    that will need removing when this product cascades.

    Mirrors `delete_evidence`'s own collection, which is the reference here:
    each Evidence's legacy primary `file_name` plus every file in its
    Evidence File list, not the primary alone. Without the Evidence File
    list, every screenshot beyond the first survives its database row."""
    files: list[str] = []
    for fw in product.frameworks:
        for ctrl in fw.controls:
            for ev in ctrl.evidence:
                files.extend({ef.file_name for ef in ev.files})
                files.append(ev.file_name)
    return files


@router.get("", response_model=list[ProductResponse])
def list_products(db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    return db.query(Product).order_by(Product.name).all()


@router.post("", response_model=ProductResponse, status_code=201)
def create_product(payload: ProductCreate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    product = Product(**payload.model_dump())
    db.add(product)
    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        raise HTTPException(status_code=409, detail=f"Product '{payload.name}' already exists")
    db.refresh(product)
    return product


@router.get("/{product_id}", response_model=ProductResponse)
def get_product(product_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Product not found")
    return product


@router.patch("/{product_id}", response_model=ProductResponse)
def update_product(product_id: int, payload: ProductUpdate, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Product not found")
    if payload.name is not None:
        product.name = payload.name.strip()
    if payload.description is not None:
        product.description = payload.description.strip() or None
    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        raise HTTPException(status_code=409, detail=f"Another product already uses that name")
    db.refresh(product)
    return product


@router.delete("/{product_id}", status_code=204)
def delete_product(product_id: int, db: Session = Depends(get_db), user: User = Depends(require_admin)):
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Product not found")
    file_names = _collect_evidence_files(product)
    db.delete(product)
    try:
        db.commit()
    except IntegrityError as exc:
        # Deleting a Product cascades down through its Frameworks and
        # Controls. Nothing left in this schema should still block that —
        # agent_tasks.control_id (once the last blocking reference) now
        # detaches with ON DELETE SET NULL rather than refusing the delete.
        # This is kept as a safety net for any future reference Postgres
        # refuses, so it fails as a clean 409 rather than an unhandled
        # server error.
        db.rollback()
        raise HTTPException(
            status_code=409,
            detail="A control under this product is still referenced elsewhere and cannot be deleted.",
        ) from exc
    delete_files(file_names)
    return Response(status_code=204)
