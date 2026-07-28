from datetime import datetime, timedelta, date, timezone

from fastapi import APIRouter, Depends, Query
from sqlalchemy import func
from sqlalchemy.orm import Session

from app.auth import User
from app.database import get_db
from app.models.usage_log import UsageLog
from app.rbac import require_admin
from app.schemas.usage import (
    UsageByModel,
    UsageDayPoint,
    UsageLogRow,
    UsageSummary,
)

router = APIRouter(prefix="/usage", tags=["Usage"])


def _now_utc() -> datetime:
    return datetime.now(timezone.utc)


def _aggregate(db: Session, since: datetime | None = None) -> dict:
    q = db.query(
        func.count(UsageLog.id),
        func.coalesce(func.sum(UsageLog.input_tokens), 0),
        func.coalesce(func.sum(UsageLog.output_tokens), 0),
        func.coalesce(func.sum(UsageLog.total_tokens), 0),
        func.coalesce(func.sum(UsageLog.llm_calls), 0),
        func.coalesce(func.sum(UsageLog.cost_usd), 0.0),
    )
    if since is not None:
        q = q.filter(UsageLog.created_at >= since)
    runs, in_t, out_t, tot_t, calls, cost = q.one()
    return {
        "runs": int(runs or 0),
        "input_tokens": int(in_t or 0),
        "output_tokens": int(out_t or 0),
        "total_tokens": int(tot_t or 0),
        "llm_calls": int(calls or 0),
        "cost_usd": float(cost or 0.0),
    }


@router.get("/summary", response_model=UsageSummary)
def usage_summary(db: Session = Depends(get_db), user: User = Depends(require_admin)):
    now = _now_utc()
    today_start = datetime(now.year, now.month, now.day, tzinfo=timezone.utc)

    total = _aggregate(db)
    last_7 = _aggregate(db, since=now - timedelta(days=7))
    last_30 = _aggregate(db, since=now - timedelta(days=30))
    today = _aggregate(db, since=today_start)

    return UsageSummary(
        total_runs=total["runs"],
        total_input_tokens=total["input_tokens"],
        total_output_tokens=total["output_tokens"],
        total_tokens=total["total_tokens"],
        total_llm_calls=total["llm_calls"],
        total_cost_usd=round(total["cost_usd"], 6),
        last_7_days_cost_usd=round(last_7["cost_usd"], 6),
        last_7_days_tokens=last_7["total_tokens"],
        last_7_days_runs=last_7["runs"],
        last_30_days_cost_usd=round(last_30["cost_usd"], 6),
        last_30_days_tokens=last_30["total_tokens"],
        last_30_days_runs=last_30["runs"],
        today_cost_usd=round(today["cost_usd"], 6),
        today_runs=today["runs"],
    )


@router.get("/timeseries", response_model=list[UsageDayPoint])
def usage_timeseries(
    days: int = Query(default=30, ge=1, le=180),
    db: Session = Depends(get_db),
    user: User = Depends(require_admin),
):
    """One bucket per calendar day for the last ``days`` days.
    Days with no runs are returned with zeros so the chart line stays continuous."""
    now = _now_utc()
    since = datetime(now.year, now.month, now.day, tzinfo=timezone.utc) - timedelta(days=days - 1)

    rows = (
        db.query(
            func.date(UsageLog.created_at).label("day"),
            func.coalesce(func.sum(UsageLog.input_tokens), 0),
            func.coalesce(func.sum(UsageLog.output_tokens), 0),
            func.coalesce(func.sum(UsageLog.cost_usd), 0.0),
            func.count(UsageLog.id),
        )
        .filter(UsageLog.created_at >= since)
        .group_by(func.date(UsageLog.created_at))
        .order_by(func.date(UsageLog.created_at))
        .all()
    )

    by_day = {r[0]: r for r in rows}

    out: list[UsageDayPoint] = []
    for i in range(days):
        day: date = (since + timedelta(days=i)).date()
        r = by_day.get(day)
        if r is None:
            out.append(UsageDayPoint(
                date=day.isoformat(),
                input_tokens=0,
                output_tokens=0,
                cost_usd=0.0,
                runs=0,
            ))
        else:
            out.append(UsageDayPoint(
                date=day.isoformat(),
                input_tokens=int(r[1] or 0),
                output_tokens=int(r[2] or 0),
                cost_usd=round(float(r[3] or 0.0), 6),
                runs=int(r[4] or 0),
            ))
    return out


@router.get("/by-model", response_model=list[UsageByModel])
def usage_by_model(db: Session = Depends(get_db), user: User = Depends(require_admin)):
    rows = (
        db.query(
            UsageLog.model,
            func.count(UsageLog.id),
            func.coalesce(func.sum(UsageLog.input_tokens), 0),
            func.coalesce(func.sum(UsageLog.output_tokens), 0),
            func.coalesce(func.sum(UsageLog.total_tokens), 0),
            func.coalesce(func.sum(UsageLog.cost_usd), 0.0),
        )
        .group_by(UsageLog.model)
        .order_by(func.sum(UsageLog.cost_usd).desc())
        .all()
    )
    return [
        UsageByModel(
            model=r[0],
            runs=int(r[1] or 0),
            input_tokens=int(r[2] or 0),
            output_tokens=int(r[3] or 0),
            total_tokens=int(r[4] or 0),
            cost_usd=round(float(r[5] or 0.0), 6),
        )
        for r in rows
    ]


@router.get("/recent", response_model=list[UsageLogRow])
def recent_usage(
    limit: int = Query(default=20, ge=1, le=100),
    db: Session = Depends(get_db),
    user: User = Depends(require_admin),
):
    return (
        db.query(UsageLog)
        .order_by(UsageLog.created_at.desc())
        .limit(limit)
        .all()
    )
