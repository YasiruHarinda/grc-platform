"""
Coverage for `POST /api/controls` resolving its parent Framework before
inserting.

Mirrors `create_framework`'s own check on its parent Product, and
`create_evidence`'s check on its parent Control (issue #16): naming a
parent that doesn't exist is the caller's mistake, so it must come back as
a clean not-found rather than the raw foreign-key `IntegrityError` the
database itself would raise.
"""
from app.models.control import Control

from tests.conftest import make_control


def test_create_control_under_an_unknown_framework_is_a_bad_request_not_a_server_error(
    db_session, admin_client
):
    """Naming a Framework that doesn't exist must come back as 404, not the
    raw IntegrityError a foreign-key violation would otherwise surface as."""
    response = admin_client.post(
        "/api/controls",
        json={
            "framework_id": 999999999,
            "control_ref": "C-1",
            "title": "Test Control",
        },
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Framework not found"
    assert db_session.query(Control).count() == 0


def test_create_control_under_an_existing_framework_still_works(db_session, admin_client):
    """The new parent check must not get in the way of the ordinary case."""
    existing_control = make_control(db_session)
    framework_id = existing_control.framework_id

    response = admin_client.post(
        "/api/controls",
        json={
            "framework_id": framework_id,
            "control_ref": "C-2",
            "title": "Another Control",
        },
    )

    assert response.status_code == 201
    body = response.json()
    assert body["framework_id"] == framework_id
    assert body["control_ref"] == "C-2"
    assert db_session.query(Control).filter(Control.framework_id == framework_id).count() == 2
