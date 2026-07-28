"""Usage route day/window boundaries must be computed in UTC, not in
whatever timezone the database session happens to be using.

`usage_summary` and `usage_timeseries` both build a "since" cutoff from
`_now_utc()` and compare it against `UsageLog.created_at` (a
`timestamptz` column). If that cutoff is a *naive* datetime, Postgres
has no timezone to read off it, so it falls back to interpreting the
naive value in the session's `TimeZone` setting rather than UTC -- which
silently shifts the boundary by that session's UTC offset. A session
that happens to be UTC (the common case, including this suite's default
container) hides the bug entirely, since the naive and the intended UTC
values agree.

Each test below pins the test connection's session timezone to
`America/New_York` (UTC-4 in July) before hitting the route, specifically
to make a naive boundary disagree with the correct UTC one. That turns
"is the boundary timezone-aware UTC" into an assertion on the response
body instead of something that only happens to pass because of how the
test database is configured.
"""
from datetime import datetime, timedelta, timezone

from sqlalchemy import text

from app.models.usage_log import UsageLog


def _log(run_id: str, created_at: datetime, cost_usd: float = 1.0) -> UsageLog:
    return UsageLog(
        run_id=run_id,
        model="test-model",
        provider="test",
        input_tokens=1,
        output_tokens=1,
        total_tokens=2,
        llm_calls=1,
        cost_usd=cost_usd,
        created_at=created_at,
    )


def test_usage_summary_today_boundary_is_utc_not_session_timezone(db_session, admin_client):
    # Force the test connection off UTC so a naive "today start" would
    # land on a visibly different instant than the correct UTC one.
    db_session.execute(text("SET TIME ZONE 'America/New_York'"))

    now = datetime.now(timezone.utc)
    today_utc_midnight = datetime(now.year, now.month, now.day, tzinfo=timezone.utc)

    # One row just before the UTC day boundary (yesterday), one just after
    # (today). Only the second should count toward "today".
    db_session.add(_log("before-midnight-utc", today_utc_midnight - timedelta(seconds=1), cost_usd=5.0))
    db_session.add(_log("after-midnight-utc", today_utc_midnight + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    response = admin_client.get("/api/usage/summary")
    assert response.status_code == 200
    body = response.json()

    assert body["today_runs"] == 1
    assert body["today_cost_usd"] == 2.0


def test_usage_timeseries_since_cutoff_excludes_row_before_window(db_session, admin_client):
    # Deliberately doesn't use SET TIME ZONE here (unlike the summary test
    # above): /timeseries also groups rows by `func.date(UsageLog.created_at)`,
    # which Postgres evaluates in the *session* timezone -- a separate,
    # pre-existing behaviour this ticket doesn't touch. Forcing a non-UTC
    # session here would shift which calendar-day bucket a row lands in and
    # make this test flaky for reasons unrelated to the "since" cutoff it's
    # meant to check. Summing `runs` across every returned point (rather than
    # asserting on one specific day) keeps the assertion about the cutoff
    # itself: whether the window's start boundary is drawn in the right
    # place, not which bucket a row that's inside it gets filed under.
    now = datetime.now(timezone.utc)
    today_utc_midnight = datetime(now.year, now.month, now.day, tzinfo=timezone.utc)

    days = 3
    since = today_utc_midnight - timedelta(days=days - 1)

    db_session.add(_log("before-window", since - timedelta(seconds=1)))
    db_session.add(_log("in-window", since + timedelta(seconds=1)))
    db_session.commit()

    response = admin_client.get("/api/usage/timeseries", params={"days": days})
    assert response.status_code == 200
    points = response.json()

    assert len(points) == days
    total_runs = sum(p["runs"] for p in points)
    assert total_runs == 1
