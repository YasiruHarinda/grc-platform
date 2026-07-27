"""
Checklist 4.6 mandatory security headers: every API response must carry
`X-Content-Type-Options`, `Content-Security-Policy`, and
`Strict-Transport-Security` with the exact values below (see issue #74).

These are additive/safe headers that restrict no content sources, so a plain
200 from any authenticated route is enough to prove they're present.
"""


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
