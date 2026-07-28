"""
Smoke tests for the test harness itself: proves the FastAPI app boots under
the test environment, and that the `get_db` / `get_current_user` overrides
from conftest.py actually take effect over HTTP.
"""


def test_list_evidence_as_engineer_returns_200(engineer_client):
    response = engineer_client.get("/api/evidence")

    assert response.status_code == 200
    assert response.json() == []


def test_list_evidence_as_admin_returns_200(admin_client):
    response = admin_client.get("/api/evidence")

    assert response.status_code == 200
    assert response.json() == []


def test_list_evidence_without_auth_override_is_rejected(client):
    """`client` only overrides `get_db` — `get_current_user` is untouched, so
    a request with no Bearer token still hits the real auth dependency and is
    refused, without making any network call to Asgardeo."""
    response = client.get("/api/evidence")

    assert response.status_code == 401
