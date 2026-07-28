"""
Deleting a Control an Agent Task still points at -- directly, or via the
Framework or Product above it -- must fail cleanly with 409 Conflict, not
crash with an unhandled 500.

`agent_tasks.control_id` is a foreign key to `controls.id` with no ON DELETE
rule, so Postgres refuses to delete a referenced Control (RESTRICT). A
Framework/Product delete cascades down to its Controls, so it hits the same
refusal. Each delete route has to catch that refusal and turn it into a 409,
the same way create/update on Frameworks and Products translate their own
IntegrityErrors. Without the catch the IntegrityError propagates out of the
route as a server error, and the caller gets no usable answer to a request
that can never succeed.

The reference lingers even for finished work: nothing clears `control_id`
once a task is completed/failed/cancelled, so a Control -- or anything above
it -- can be undeletable because of a task that ran months ago.

The 409 cases below also carry Evidence with more than one file, and assert
both the primary and secondary blobs are still fetchable after the refusal.
Each delete route collects its evidence blobs *before* committing but only
calls `delete_file` *after* a successful commit (see e.g. `products.py`'s
`delete_product`: `file_names = _collect_evidence_files(product)` up front,
`for name in file_names: delete_file(name)` only after the `try/except
IntegrityError` around `db.commit()`). A rejected delete -- one that hits the
`except IntegrityError` branch -- must never reach that loop. Without Evidence
attached to the referenced Control, a 409 test can't tell a route that gets
this ordering right from one that deletes blobs unconditionally regardless of
whether the commit actually went through.
"""
import httpx

from app.models.agent_task import AgentTask
from app.models.control import Control
from app.models.framework import Framework
from app.models.product import Product
from app.storage.blob_storage import get_signed_url

from tests.conftest import build_evidence, make_control


def _task_against(control_id: int, *, status: str) -> AgentTask:
    return AgentTask(
        user_email="engineer@example.com",
        prompt="check this control",
        status=status,
        control_id=control_id,
    )


def _ids_for(db_session, control: Control) -> tuple[int, int]:
    """(framework_id, product_id) for the chain above a Control."""
    framework = db_session.get(Framework, control.framework_id)
    return framework.id, framework.product_id


def _signed_urls_for(files) -> list[str]:
    return [get_signed_url(ef.file_name) for ef in files]


def _assert_all_still_fetchable(signed_urls: list[str]) -> None:
    for url in signed_urls:
        assert httpx.get(url).status_code == 200


def test_deleting_a_control_still_referenced_by_a_task_returns_409(db_session, admin_client):
    control = make_control(db_session)
    _evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    db_session.add(_task_against(control.id, status="running"))
    db_session.commit()

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 409
    # A refused delete must be a no-op, not a partial one: the Control stays.
    assert db_session.get(Control, control.id) is not None
    # ...and neither does its Evidence's storage: a rejected delete must
    # clean up nothing, since it committed nothing.
    _assert_all_still_fetchable(signed_urls)


def test_a_finished_task_still_blocks_deleting_its_control(db_session, admin_client):
    control = make_control(db_session)
    _evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    db_session.add(_task_against(control.id, status="completed"))
    db_session.commit()

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 409
    assert db_session.get(Control, control.id) is not None
    _assert_all_still_fetchable(signed_urls)


def test_deleting_a_framework_whose_control_is_referenced_returns_409(db_session, admin_client):
    control = make_control(db_session)
    framework_id, _ = _ids_for(db_session, control)
    _evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    db_session.add(_task_against(control.id, status="completed"))
    db_session.commit()

    response = admin_client.delete(f"/api/frameworks/{framework_id}")

    assert response.status_code == 409
    # Neither the Framework nor its Control may be partially removed.
    assert db_session.get(Framework, framework_id) is not None
    assert db_session.get(Control, control.id) is not None
    _assert_all_still_fetchable(signed_urls)


def test_deleting_a_product_whose_control_is_referenced_returns_409(db_session, admin_client):
    control = make_control(db_session)
    _, product_id = _ids_for(db_session, control)
    _evidence, files = build_evidence(
        db_session,
        ("primary.png", b"primary screenshot"),
        ("secondary.png", b"secondary screenshot"),
        control_id=control.id,
    )
    signed_urls = _signed_urls_for(files)
    db_session.add(_task_against(control.id, status="cancelled"))
    db_session.commit()

    response = admin_client.delete(f"/api/products/{product_id}")

    assert response.status_code == 409
    assert db_session.get(Product, product_id) is not None
    assert db_session.get(Control, control.id) is not None
    _assert_all_still_fetchable(signed_urls)


def test_deleting_a_control_no_task_references_still_succeeds(db_session, admin_client):
    control = make_control(db_session)

    response = admin_client.delete(f"/api/controls/{control.id}")

    assert response.status_code == 204
    assert db_session.get(Control, control.id) is None


def test_deleting_a_framework_with_no_referenced_control_still_succeeds(db_session, admin_client):
    control = make_control(db_session)
    framework_id, _ = _ids_for(db_session, control)

    response = admin_client.delete(f"/api/frameworks/{framework_id}")

    assert response.status_code == 204
    assert db_session.get(Framework, framework_id) is None


def test_deleting_a_product_with_no_referenced_control_still_succeeds(db_session, admin_client):
    control = make_control(db_session)
    _, product_id = _ids_for(db_session, control)

    response = admin_client.delete(f"/api/products/{product_id}")

    assert response.status_code == 204
    assert db_session.get(Product, product_id) is None
