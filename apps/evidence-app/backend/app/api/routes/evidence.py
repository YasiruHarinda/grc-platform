from fastapi import APIRouter, Depends, HTTPException, UploadFile, File, Form
from sqlalchemy.orm import Session, selectinload
from app.auth import User, get_current_user
from app.database import get_db
from app.models.control import Control
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.models.submission import Submission
from app.schemas.evidence import EvidenceResponse, EvidenceUpdate
from app.storage.blob_storage import save_file, delete_file

router = APIRouter(prefix="/evidence", tags=["Evidence"])


def _authorize_evidence_access(evidence: Evidence | None, user: User) -> None:
    """Shared owner-or-admin check for evidence-linked reads: only the
    Evidence's owner or an Admin may access it.

    Also owns the "evidence is missing" case, so callers that only have an
    Evidence id to resolve (e.g. app.api.routes.submissions, resolving a
    Submission's evidence_id) get a 404 rather than treating a dangling
    reference as a 403.
    """
    if evidence is None:
        raise HTTPException(status_code=404, detail="Evidence not found")
    if user.role != "admin" and evidence.created_by != user.email:
        raise HTTPException(status_code=403)


@router.get("", response_model=list[EvidenceResponse])
def list_evidence(db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    q = db.query(Evidence).options(selectinload(Evidence.files))
    if user.role != "admin":
        q = q.filter(Evidence.created_by == user.email)
    return q.order_by(Evidence.id.desc()).all()


@router.post("", response_model=EvidenceResponse, status_code=201)
def create_evidence(
    title: str = Form(...),
    control_id: int = Form(...),
    description: str | None = Form(default=None),
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
):
    # Resolve the parent Control before uploading anything. Naming a Control
    # that doesn't exist is a bad request, not a server failure: letting it
    # fall through to the foreign key would surface as a 500 telling the
    # caller to retry, which can never succeed. Doing this first also means a
    # doomed request never uploads a blob that would then need cleaning up.
    if db.query(Control).filter(Control.id == control_id).first() is None:
        raise HTTPException(status_code=404, detail="Control not found")

    file_name, file_url = save_file(file)
    try:
        evidence = Evidence(
            title=title,
            description=description,
            file_name=file_name,
            file_url=file_url,
            control_id=control_id,
            created_by=user.email,
        )
        db.add(evidence)
        # Flush (not commit) to assign evidence.id, so the EvidenceFile and
        # Submission below can reference it while all three rows still live
        # in the same uncommitted transaction.
        db.flush()

        db.add(EvidenceFile(
            evidence_id=evidence.id,
            file_name=file_name,
            file_url=file_url,
            sort_order=0,
        ))
        db.add(Submission(
            evidence_id=evidence.id,
            submitted_by=user.email,
            status="pending",
            notes=f"Manual upload via Submit page. {description or ''}".strip(),
        ))
        db.commit()
    except Exception:
        db.rollback()
        # The upload happens before the transaction and isn't something the
        # database can roll back, so a failed write would otherwise strand
        # the blob we just uploaded with nothing left to reference it. Clean
        # it up explicitly so a failed submission leaves nothing behind in
        # storage either, not just in the database.
        delete_file(file_name)
        raise HTTPException(
            status_code=500,
            detail="Failed to create evidence. Please try again.",
        )

    db.refresh(evidence)
    return evidence


@router.delete("/files/{file_id}", status_code=204)
def delete_evidence_file(file_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    ef = db.query(EvidenceFile).filter(EvidenceFile.id == file_id).first()
    if not ef:
        raise HTTPException(status_code=404, detail="File not found")
    evidence_id = ef.evidence_id
    deleted_file_name = ef.file_name
    evidence = db.query(Evidence).filter(Evidence.id == evidence_id).first()
    _authorize_evidence_access(evidence, user)
    # The Evidence's own file_name/file_url is a separate, legacy reference
    # alongside the EvidenceFile list (see ADR/spec for evidence.py:95): it
    # is "primary" exactly when it still points at the file being deleted.
    was_primary = evidence is not None and ef.file_name == evidence.file_name

    # Read the survivor (if any) before mutating anything, so the decision --
    # repoint the primary, delete the now-empty parent, or neither -- is made
    # from a consistent, pre-mutation snapshot rather than a query that would
    # otherwise see the deletion below once it autoflushes.
    remaining = (
        db.query(EvidenceFile)
        .filter(EvidenceFile.evidence_id == evidence_id, EvidenceFile.id != file_id)
        .order_by(EvidenceFile.sort_order)
        .all()
    )

    # Every mutation happens in one transaction with a single commit, the
    # same all-or-nothing principle create_evidence uses: a failed commit
    # must leave nothing changed, rather than leaving an Evidence pointing at
    # a blob that's already gone, or an Evidence with zero files. Blobs are
    # only deleted after this commit succeeds (below) -- deleting first, the
    # way the previous two-commit version did, strands storage even when
    # just the first commit fails.
    parent_deleted = False
    legacy_blob_name: str | None = None
    db.delete(ef)
    if not remaining:
        if evidence:
            parent_deleted = True
            legacy_blob_name = evidence.file_name
            db.delete(evidence)
    elif was_primary:
        # Repoint at the next surviving file rather than clearing the
        # reference: nulling it would push the inconsistency into every
        # reader that expects it populated. The survivor is the remaining
        # file with the lowest sort_order, i.e. the first file in the order
        # the gallery is meant to present them.
        survivor = remaining[0]
        evidence.file_name = survivor.file_name
        evidence.file_url = survivor.file_url
    db.commit()

    delete_file(deleted_file_name)
    if parent_deleted:
        delete_file(legacy_blob_name)


@router.get("/{evidence_id}", response_model=EvidenceResponse)
def get_evidence(evidence_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    evidence = db.query(Evidence).options(selectinload(Evidence.files)).filter(Evidence.id == evidence_id).first()
    _authorize_evidence_access(evidence, user)
    return evidence


@router.patch("/{evidence_id}", response_model=EvidenceResponse)
def update_evidence(evidence_id: int, body: EvidenceUpdate, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    evidence = (
        db.query(Evidence)
        .options(selectinload(Evidence.files))
        .filter(Evidence.id == evidence_id)
        .first()
    )
    _authorize_evidence_access(evidence, user)
    evidence.description = body.description
    db.commit()
    db.refresh(evidence)
    return evidence


@router.delete("/{evidence_id}", status_code=204)
def delete_evidence(evidence_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    evidence = db.query(Evidence).options(selectinload(Evidence.files)).filter(Evidence.id == evidence_id).first()
    _authorize_evidence_access(evidence, user)
    file_names = {ef.file_name for ef in evidence.files}
    file_names.add(evidence.file_name)
    db.delete(evidence)
    db.commit()
    # Delete blobs only after the row is gone, matching delete_control/
    # delete_framework/delete_product. A failed commit must not strand
    # Evidence rows pointing at blobs that were already removed.
    for fn in file_names:
        delete_file(fn)