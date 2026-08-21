"""
Tests for `GET /api/runner-config`.

This endpoint exists so the Runner can learn its Asgardeo org and client ID
*before* it has signed in -- see the docstring on the route itself
(app/api/routes/runner_config.py) for why that means it must be
unauthenticated and why the two values it returns are not secrets.

No client-id-bearing fixture is used here on purpose: the point of these
tests is that the endpoint answers with no Authorization header at all, so
they hit the app directly with a plain `TestClient`, the same way
`app.config.settings` is asserted against directly rather than through a
fixture that might quietly add auth.
"""
from fastapi.testclient import TestClient

from app.config import settings
from app.main import app


def test_runner_config_returns_200_with_no_authorization_header():
    client = TestClient(app)

    response = client.get("/api/runner-config")

    assert response.status_code == 200


def test_runner_config_body_has_exactly_the_two_expected_keys():
    """Pins the response shape so a future edit cannot widen it by accident
    -- this endpoint is unauthenticated, so anything added here is public by
    definition, and that has to be a deliberate decision each time."""
    client = TestClient(app)

    response = client.get("/api/runner-config")

    assert set(response.json().keys()) == {"asgardeo_org", "asgardeo_client_id"}


def test_runner_config_values_come_from_settings():
    client = TestClient(app)

    response = client.get("/api/runner-config")

    body = response.json()
    assert body["asgardeo_org"] == settings.ASGARDEO_ORG
    assert body["asgardeo_client_id"] == settings.ASGARDEO_RUNNER_CLIENT_ID


def test_runner_config_client_id_is_the_runner_id_not_the_webapp_id():
    """`app/config.py` refuses to start unless the two client IDs differ
    (see test_config.py), and conftest.py's test values honour that. That
    difference is what makes this test meaningful: if the route accidentally
    served ASGARDEO_WEBAPP_CLIENT_ID instead, this would catch it, whereas a
    suite where the two IDs happened to match could not."""
    assert settings.ASGARDEO_RUNNER_CLIENT_ID != settings.ASGARDEO_WEBAPP_CLIENT_ID

    client = TestClient(app)
    response = client.get("/api/runner-config")

    body = response.json()
    assert body["asgardeo_client_id"] == settings.ASGARDEO_RUNNER_CLIENT_ID
    assert body["asgardeo_client_id"] != settings.ASGARDEO_WEBAPP_CLIENT_ID
