from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from app.api.routes.evidence import _authorize_evidence_access
from app.auth import User, get_current_user
from app.database import get_db
from app.models.evidence import Evidence
from app.models.submission import Submission
from app.schemas.submission import SubmissionCreate, SubmissionResponse, SubmissionStatusUpdate

router = APIRouter(prefix="/submissions", tags=["Submissions"])

VALID_STATUSES = {"pending", "approved", "rejected"}


@router.get("", response_model=list[SubmissionResponse])
def list_submissions(db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    q = db.query(Submission)
    if user.role != "admin":
        q = q.join(Evidence, Submission.evidence_id == Evidence.id).filter(Evidence.created_by == user.email)
    return q.order_by(Submission.id.desc()).all()


@router.post("", response_model=SubmissionResponse, status_code=201)
def create_submission(payload: SubmissionCreate, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    evidence = db.query(Evidence).filter(Evidence.id == payload.evidence_id).first()
    _authorize_evidence_access(evidence, user)
    submission = Submission(evidence_id=payload.evidence_id, submitted_by=user.email, notes=payload.notes)
    db.add(submission)
    db.commit()
    db.refresh(submission)
    return submission


@router.get("/{submission_id}", response_model=SubmissionResponse)
def get_submission(submission_id: int, db: Session = Depends(get_db), user: User = Depends(get_current_user)):
    submission = db.query(Submission).filter(Submission.id == submission_id).first()
    if not submission:
        raise HTTPException(status_code=404, detail="Submission not found")
    if user.role != "admin":
        evidence = db.query(Evidence).filter(Evidence.id == submission.evidence_id).first()
        _authorize_evidence_access(evidence, user)
    return submission


@router.patch("/{submission_id}", response_model=SubmissionResponse)
def update_submission_status(
    submission_id: int,
    payload: SubmissionStatusUpdate,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
):
    submission = db.query(Submission).filter(Submission.id == submission_id).first()
    if not submission:
        raise HTTPException(status_code=404, detail="Submission not found")
    if user.role != "admin":
        evidence = db.query(Evidence).filter(Evidence.id == submission.evidence_id).first()
        _authorize_evidence_access(evidence, user)
    if payload.status not in VALID_STATUSES:
        raise HTTPException(status_code=400, detail=f"Status must be one of: {', '.join(sorted(VALID_STATUSES))}")
    submission.status = payload.status
    db.commit()
    db.refresh(submission)
    return submission