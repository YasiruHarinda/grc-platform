"""Usage Reset: an Admin can record a moment the Cost & Usage summary starts
counting from again, without deleting any Usage Log row.

Two real failure modes this guards against, both silent if they slip
through:

- A reset that quietly deletes or otherwise loses Usage Log rows. The whole
  point of a reset over a manual database delete is that the spend history
  survives for audit -- a test asserting the row count is unchanged is the
  thing that actually enforces that promise, not just the docs.
- A "Last 7 Days" (or 30, or Today) window that ignores the reset and keeps
  using its own start boundary. When a reset falls inside a window, the
  window must use whichever boundary is *later* -- otherwise it can count a
  pre-reset run the total above it has already excluded, and a window
  reporting more spend than the total is the one place this feature could
  visibly break the page.

Written in the same style as test_usage_boundaries.py: seed UsageLog rows
with explicit `created_at` timestamps, hit the route, assert on the
response body -- plus, here, row counts and the recorded reset itself,
since "nothing is deleted" and "who pressed reset" are both promises that
can only be checked against the database.
"""
from datetime import datetime, timedelta, timezone

from app.models.usage_log import UsageLog
from app.models.usage_reset import UsageReset


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


def test_reset_endpoint_requires_admin_role(db_session, engineer_client):
    response = engineer_client.post("/api/usage/reset")
    assert response.status_code == 403
    assert db_session.query(UsageReset).count() == 0


def test_reset_endpoint_records_who_and_when(db_session, admin_client, admin_user):
    before = datetime.now(timezone.utc)
    response = admin_client.post("/api/usage/reset")
    after = datetime.now(timezone.utc)

    assert response.status_code == 200
    body = response.json()
    counting_since = datetime.fromisoformat(body["counting_since"])
    assert before <= counting_since <= after

    resets = db_session.query(UsageReset).all()
    assert len(resets) == 1
    assert resets[0].reset_by == admin_user.email
    assert resets[0].effective_at == counting_since


def test_summary_with_no_reset_ever_recorded_is_unaffected(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("only-run", now, cost_usd=3.0))
    db_session.commit()

    response = admin_client.get("/api/usage/summary")
    assert response.status_code == 200
    body = response.json()

    assert body["total_runs"] == 1
    assert body["total_cost_usd"] == 3.0
    assert body["counting_since"] is None


def test_summary_after_reset_reports_zero_everywhere(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    reset_response = admin_client.post("/api/usage/reset")
    assert reset_response.status_code == 200

    response = admin_client.get("/api/usage/summary")
    assert response.status_code == 200
    body = response.json()

    assert body["total_runs"] == 0
    assert body["total_tokens"] == 0
    assert body["total_llm_calls"] == 0
    assert body["total_cost_usd"] == 0
    assert body["today_runs"] == 0
    assert body["today_cost_usd"] == 0
    assert body["last_7_days_runs"] == 0
    assert body["last_7_days_cost_usd"] == 0
    assert body["last_30_days_runs"] == 0
    assert body["last_30_days_cost_usd"] == 0
    assert body["counting_since"] is not None


def test_run_after_reset_is_counted_with_only_its_own_figures(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-reset", datetime.now(timezone.utc) + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    body = admin_client.get("/api/usage/summary").json()

    assert body["total_runs"] == 1
    assert body["total_cost_usd"] == 2.0


def test_run_before_reset_is_never_counted(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    body = admin_client.get("/api/usage/summary").json()

    assert body["total_runs"] == 0
    assert body["total_cost_usd"] == 0


def test_window_never_reports_more_than_total_when_reset_falls_inside_it(db_session, admin_client):
    # Sits inside the Last 7 Days window (3 days ago) but predates the
    # reset below. A window that used only its own 7-day start -- ignoring
    # the reset -- would still count this row, while the total (correctly
    # cut off at the reset) would not: exactly the "window > total" bug
    # this feature must never produce.
    inside_window_before_reset = datetime.now(timezone.utc) - timedelta(days=3)
    db_session.add(_log("inside-window-before-reset", inside_window_before_reset, cost_usd=9.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-reset", datetime.now(timezone.utc) + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    body = admin_client.get("/api/usage/summary").json()

    assert body["total_runs"] == 1
    assert body["total_cost_usd"] == 2.0
    assert body["last_7_days_runs"] == 1
    assert body["last_7_days_cost_usd"] == 2.0
    assert body["last_7_days_cost_usd"] <= body["total_cost_usd"]


def test_two_resets_in_sequence_leave_the_later_one_in_effect(db_session, admin_client):
    admin_client.post("/api/usage/reset")

    # Real wall-clock order, not an artificial future offset: this row must
    # land strictly between the two POSTs, and the only thing that
    # guarantees that is executing it between them.
    db_session.add(_log("between-the-two-resets", datetime.now(timezone.utc), cost_usd=4.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-second-reset", datetime.now(timezone.utc), cost_usd=1.0))
    db_session.commit()

    body = admin_client.get("/api/usage/summary").json()

    assert body["total_runs"] == 1
    assert body["total_cost_usd"] == 1.0


def test_reset_does_not_delete_any_usage_log_row(db_session, admin_client):
    now = datetime.now(timezone.utc)
    for i in range(5):
        db_session.add(_log(f"run-{i}", now - timedelta(days=i)))
    db_session.commit()

    count_before = db_session.query(UsageLog).count()

    response = admin_client.post("/api/usage/reset")
    assert response.status_code == 200

    count_after = db_session.query(UsageLog).count()
    assert count_after == count_before


# --- Ticket #127: the daily chart, the by-model breakdown and the recent
# runs list honour the same cutoff as the summary above. ---


def test_timeseries_with_no_reset_ever_recorded_is_unaffected(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("only-run", now, cost_usd=3.0))
    db_session.commit()

    response = admin_client.get("/api/usage/timeseries", params={"days": 5})
    assert response.status_code == 200
    points = response.json()

    assert len(points) == 5
    assert sum(p["runs"] for p in points) == 1


def test_timeseries_after_reset_is_zero_but_keeps_its_length(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    response = admin_client.get("/api/usage/timeseries", params={"days": 5})
    assert response.status_code == 200
    points = response.json()

    # The axis does not shrink: still exactly the number of days asked for,
    # every one of them zero, including the days before the cutoff.
    assert len(points) == 5
    assert all(p["runs"] == 0 for p in points)
    assert all(p["cost_usd"] == 0 for p in points)


def test_run_after_reset_appears_in_timeseries(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-reset", datetime.now(timezone.utc) + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    response = admin_client.get("/api/usage/timeseries", params={"days": 5})
    points = response.json()

    assert len(points) == 5
    assert sum(p["runs"] for p in points) == 1
    assert sum(p["cost_usd"] for p in points) == 2.0


def test_by_model_with_no_reset_ever_recorded_is_unaffected(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("only-run", now, cost_usd=3.0))
    db_session.commit()

    response = admin_client.get("/api/usage/by-model")
    assert response.status_code == 200
    body = response.json()

    assert len(body) == 1
    assert body[0]["model"] == "test-model"
    assert body[0]["cost_usd"] == 3.0


def test_by_model_reports_no_models_immediately_after_reset(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    response = admin_client.get("/api/usage/by-model")
    assert response.status_code == 200
    assert response.json() == []


def test_run_after_reset_appears_in_by_model(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-reset", datetime.now(timezone.utc) + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    body = admin_client.get("/api/usage/by-model").json()

    assert len(body) == 1
    assert body[0]["cost_usd"] == 2.0


def test_recent_with_no_reset_ever_recorded_is_unaffected(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("only-run", now, cost_usd=3.0))
    db_session.commit()

    response = admin_client.get("/api/usage/recent")
    assert response.status_code == 200
    body = response.json()

    assert len(body) == 1
    assert body[0]["run_id"] == "only-run"


def test_recent_is_empty_immediately_after_reset(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    response = admin_client.get("/api/usage/recent")
    assert response.status_code == 200
    assert response.json() == []


def test_run_after_reset_appears_in_recent(db_session, admin_client):
    now = datetime.now(timezone.utc)
    db_session.add(_log("before-reset", now, cost_usd=5.0))
    db_session.commit()

    admin_client.post("/api/usage/reset")

    db_session.add(_log("after-reset", datetime.now(timezone.utc) + timedelta(seconds=1), cost_usd=2.0))
    db_session.commit()

    body = admin_client.get("/api/usage/recent").json()

    assert len(body) == 1
    assert body[0]["run_id"] == "after-reset"


def test_reset_does_not_change_usage_log_row_count_across_all_reports(db_session, admin_client):
    # The same promise as test_reset_does_not_delete_any_usage_log_row above,
    # checked again here because this ticket adds a filter to three more
    # queries -- none of them should ever touch the row count.
    now = datetime.now(timezone.utc)
    for i in range(5):
        db_session.add(_log(f"run-{i}", now - timedelta(days=i)))
    db_session.commit()
    count_before = db_session.query(UsageLog).count()

    admin_client.post("/api/usage/reset")
    admin_client.get("/api/usage/timeseries", params={"days": 5})
    admin_client.get("/api/usage/by-model")
    admin_client.get("/api/usage/recent")

    assert db_session.query(UsageLog).count() == count_before
