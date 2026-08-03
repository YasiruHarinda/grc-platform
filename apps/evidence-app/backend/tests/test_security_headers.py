"""
Checklist 4.6 mandatory security headers: every API response must carry
`X-Content-Type-Options`, `Content-Security-Policy`, and
`Strict-Transport-Security` with the exact values below (see issue #74).

These are additive/safe headers that restrict no content sources, so a plain
200 from any authenticated route is enough to prove they're present.

"Every response" includes the 500 nobody planned for. The header middleware
only sees responses the app actually returned, so an unhandled exception --
which propagates past it to Starlette's ServerErrorMiddleware, sitting
outside it -- would otherwise produce a 500 carrying none of them.
`test_unhandled_exception_response_still_carries_the_headers` pins that.
"""
import pytest
from fastapi.testclient import TestClient

from app.main import app


EXPECTED_HEADERS = {
    "X-Content-Type-Options": "nosniff",
    "Content-Security-Policy": "upgrade-insecure-requests",
    "Strict-Transport-Security": "max-age=31536000; includeSubDomains",
}


def test_api_response_carries_the_three_mandatory_security_headers(engineer_client):
    response = engineer_client.get("/api/evidence")

    assert response.status_code == 200
    assert response.headers["X-Content-Type-Options"] == "nosniff"
    assert response.headers["Content-Security-Policy"] == "upgrade-insecure-requests"
    assert (
        response.headers["Strict-Transport-Security"]
        == "max-age=31536000; includeSubDomains"
    )


def test_api_response_hsts_has_no_preload_directive(engineer_client):
    """The API's Strict-Transport-Security value deliberately omits `preload`
    — that's only on the web app's (webapp/index.js). Guards against the two
    getting unified by a future "helpful" cleanup."""
    response = engineer_client.get("/api/evidence")

    assert "preload" not in response.headers["Strict-Transport-Security"]


@pytest.fixture()
def boom_route():
    """Temporarily mount a route that raises, so the unhandled-exception path
    can be driven over real HTTP. Removed again afterwards so no other test
    ever sees it."""
    @app.get("/__boom__")
    def _boom():
        raise RuntimeError("deliberate failure, for the 500 path")

    try:
        yield "/__boom__"
    finally:
        app.router.routes[:] = [
            route for route in app.router.routes
            if getattr(route, "path", None) != "/__boom__"
        ]


def test_unhandled_exception_response_still_carries_the_headers(boom_route):
    # `raise_server_exceptions=False` makes the client behave like a real
    # deployment: it returns the 500 the server would send, instead of
    # re-raising the exception into the test.
    with TestClient(app, raise_server_exceptions=False) as client:
        response = client.get(boom_route)

    assert response.status_code == 500
    for header, value in EXPECTED_HEADERS.items():
        assert response.headers[header] == value, header


def test_unhandled_exception_response_is_otherwise_unchanged(boom_route):
    """Owning the 500 must not change what a crash looks like -- same status
    and same body Starlette produced before, only the headers are added."""
    with TestClient(app, raise_server_exceptions=False) as client:
        response = client.get(boom_route)

    assert response.status_code == 500
    assert response.text == "Internal Server Error"
