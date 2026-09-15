"""
Coverage for `POST /api/controls/bulk`, which replaces the webapp's CSV
import loop (one `POST /api/controls` call per row -- 103 of them for a real
export) with one request that creates every Control under a single
Framework atomically.

The endpoint's whole design rests on checking every row before writing any
of them, so a single unusable row in a large file can't take the rest of
the import down with it. These tests go through the HTTP endpoint only
(the spec's own testing decision: the endpoint is the one seam worth
testing here), asserting on both the response body and the rows actually
committed via `db_session` -- never on route internals.

Prior art is test_control_creation.py: the same admin/engineer client
fixtures, the same `make_control` helper for a real Framework to import
into.
"""
from app.models.control import Control

from tests.conftest import make_control


def test_bulk_create_reports_every_new_row_as_created(db_session, admin_client):
    framework_id = make_control(db_session).framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "C-10", "title": "Access reviews"},
                {"control_ref": "C-11", "title": "Change management"},
                {"control_ref": "C-12", "title": "Backups", "description": "Nightly backups are verified."},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-10", "C-11", "C-12"}
    assert body["skipped"] == 0
    assert body["rejected"] == []
    stored_refs = {
        c.control_ref
        for c in db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref.in_(["C-10", "C-11", "C-12"]))
    }
    assert stored_refs == {"C-10", "C-11", "C-12"}


def test_bulk_create_sent_twice_skips_everything_the_second_time(db_session, admin_client):
    framework_id = make_control(db_session).framework_id
    payload = {
        "framework_id": framework_id,
        "controls": [
            {"control_ref": "C-20", "title": "First control"},
            {"control_ref": "C-21", "title": "Second control"},
        ],
    }

    first = admin_client.post("/api/controls/bulk", json=payload)
    assert first.status_code == 201
    assert len(first.json()["created"]) == 2

    second = admin_client.post("/api/controls/bulk", json=payload)

    # 200, not 201: the second request created no resource.
    assert second.status_code == 200
    second_body = second.json()
    assert second_body["created"] == []
    assert second_body["skipped"] == 2
    assert second_body["rejected"] == []
    assert (
        db_session.query(Control)
        .filter(Control.framework_id == framework_id, Control.control_ref.in_(["C-20", "C-21"]))
        .count()
        == 2
    )


def test_bulk_create_mixing_new_and_stored_references_creates_only_the_new_ones(db_session, admin_client):
    existing = make_control(db_session)
    framework_id = existing.framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": existing.control_ref, "title": "Duplicate of the existing one"},
                {"control_ref": "C-30", "title": "Genuinely new"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-30"}
    assert body["skipped"] == 1
    assert body["rejected"] == []
    assert (
        db_session.query(Control)
        .filter(Control.framework_id == framework_id, Control.control_ref == existing.control_ref)
        .count()
        == 1
    )


def test_bulk_create_rejects_an_overlong_reference_by_name_and_still_creates_the_rest(db_session, admin_client):
    framework_id = make_control(db_session).framework_id
    overlong_ref = "R" * 51

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": overlong_ref, "title": "Too long a reference"},
                {"control_ref": "C-40", "title": "This one is fine"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-40"}
    assert len(body["rejected"]) == 1
    assert body["rejected"][0]["control_ref"] == overlong_ref
    assert db_session.query(Control).filter(Control.control_ref == overlong_ref).count() == 0
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-40").count() == 1


def test_bulk_create_rejects_an_overlong_description_by_name_and_still_creates_the_rest(db_session, admin_client):
    framework_id = make_control(db_session).framework_id
    overlong_description = "D" * 1001

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "C-50", "title": "Too long a description", "description": overlong_description},
                {"control_ref": "C-51", "title": "This one is fine"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-51"}
    assert len(body["rejected"]) == 1
    assert body["rejected"][0]["control_ref"] == "C-50"
    assert db_session.query(Control).filter(Control.control_ref == "C-50").count() == 0
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-51").count() == 1


def test_bulk_create_rejects_an_overlong_title_by_name_and_still_creates_the_rest(db_session, admin_client):
    framework_id = make_control(db_session).framework_id
    overlong_title = "T" * 256

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "C-60", "title": overlong_title},
                {"control_ref": "C-61", "title": "This one is fine"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-61"}
    assert len(body["rejected"]) == 1
    assert body["rejected"][0]["control_ref"] == "C-60"
    assert db_session.query(Control).filter(Control.control_ref == "C-60").count() == 0
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-61").count() == 1


def test_bulk_create_rejects_a_blank_reference_or_title_rather_than_storing_it_empty(db_session, admin_client):
    framework_id = make_control(db_session).framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "   ", "title": "Has a title but no reference"},
                {"control_ref": "C-70", "title": "   "},
                {"control_ref": "C-71", "title": "This one is fine"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert {c["control_ref"] for c in body["created"]} == {"C-71"}
    assert len(body["rejected"]) == 2
    rejected_refs = {r["control_ref"] for r in body["rejected"]}
    assert rejected_refs == {"", "C-70"}
    # The blank-reference row has no reference to name it by, so its
    # 1-based position in the request is the only handle the Admin has on
    # which line of their file to go and fix. Every rejection carries one.
    by_position = {r["row_number"]: r["control_ref"] for r in body["rejected"]}
    assert by_position == {1: "", 2: "C-70"}
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-70").count() == 0
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.title == "").count() == 0


def test_bulk_create_with_the_same_reference_twice_produces_exactly_one_control(db_session, admin_client):
    framework_id = make_control(db_session).framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "C-80", "title": "First time seeing this"},
                {"control_ref": "C-80", "title": "Second time, same reference"},
            ],
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert len(body["created"]) == 1
    assert body["created"][0]["control_ref"] == "C-80"
    assert db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-80").count() == 1


def test_bulk_create_under_an_unknown_framework_returns_not_found_and_writes_nothing(db_session, admin_client):
    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": 999999999,
            "controls": [{"control_ref": "C-90", "title": "Never should land"}],
        },
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Framework not found"
    assert db_session.query(Control).filter(Control.control_ref == "C-90").count() == 0


def test_bulk_create_over_the_row_cap_is_refused_and_writes_nothing(db_session, admin_client):
    framework_id = make_control(db_session).framework_id
    rows = [{"control_ref": f"C-{i}", "title": f"Row {i}"} for i in range(1001)]

    response = admin_client.post(
        "/api/controls/bulk",
        json={"framework_id": framework_id, "controls": rows},
    )

    # The cap lives on the schema, so this is pydantic's own validation
    # refusal: `detail` is a list of errors rather than a single string.
    # The limit still has to be visible in it, because that is what the
    # dialog shows the Admin.
    assert response.status_code == 422
    detail = response.json()["detail"]
    assert isinstance(detail, list)
    assert "1000" in str(detail)
    assert db_session.query(Control).filter(Control.framework_id == framework_id).count() == 1  # only make_control's own row


def test_bulk_create_is_refused_for_an_engineer(db_session, engineer_client):
    framework_id = make_control(db_session).framework_id

    response = engineer_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [{"control_ref": "C-100", "title": "Should not be created"}],
        },
    )

    assert response.status_code == 403
    assert db_session.query(Control).filter(Control.control_ref == "C-100").count() == 0


def test_bulk_create_is_refused_when_unauthenticated(db_session, client):
    framework_id = make_control(db_session).framework_id

    response = client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [{"control_ref": "C-101", "title": "Should not be created"}],
        },
    )

    assert response.status_code == 401
    assert db_session.query(Control).filter(Control.control_ref == "C-101").count() == 0


def test_bulk_create_when_everything_is_already_stored_succeeds_with_zero_created(db_session, admin_client):
    existing = make_control(db_session)
    framework_id = existing.framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [{"control_ref": existing.control_ref, "title": "Same reference, different title"}],
        },
    )

    # 200, not 201: nothing was created, so Created would be the wrong word.
    assert response.status_code == 200
    body = response.json()
    assert body["created"] == []
    assert body["skipped"] == 1
    assert body["rejected"] == []


def test_bulk_create_trims_surrounding_whitespace_off_every_field(db_session, admin_client):
    """Trimmed the way update_control trims, which is NOT what create_control
    does: that one stores its payload untouched. The two creation paths
    genuinely normalise differently, and changing the single endpoint to
    match is a behaviour change to an endpoint this work does not touch."""
    framework_id = make_control(db_session).framework_id

    response = admin_client.post(
        "/api/controls/bulk",
        json={
            "framework_id": framework_id,
            "controls": [
                {"control_ref": "  C-110  ", "title": "  Padded title  ", "description": "  Padded description  "}
            ],
        },
    )

    assert response.status_code == 201
    created = response.json()["created"][0]
    assert created["control_ref"] == "C-110"
    assert created["title"] == "Padded title"
    assert created["description"] == "Padded description"

    stored = db_session.query(Control).filter(Control.framework_id == framework_id, Control.control_ref == "C-110").one()
    assert stored.title == "Padded title"
    assert stored.description == "Padded description"
