"""Unit tests for `wso2_runner.oauth._email_from_id_token`.

Exercises the ID-token payload decoder purely through its external
contract — the email (or sub) it returns for a given `tokens` dict — using
real base64url-encoded JWT-shaped strings built by this file, not real
Asgardeo tokens.

Two encoding pitfalls this guards against:

  * JWT payload segments are base64url, not standard base64: they can
    contain `-` and `_`, which standard `base64.b64decode` treats as
    invalid characters and silently discards, corrupting the decode.
  * The re-padding math must add zero extra `=` when the segment length is
    already a multiple of 4, not a spurious extra padding group.

`_email_from_id_token` is deliberately best-effort (it feeds terminal
display and the switch-account guard only, never authentication), so
malformed or absent input must return None rather than raise.
"""
import base64
import json

from wso2_runner.oauth import _email_from_id_token, _state_matches


def _make_id_token(payload: dict) -> str:
    """Build a `header.payload.sig` string shaped like a real JWT,
    base64url-encoding only the payload segment — the only one
    `_email_from_id_token` reads. Header and signature are placeholders."""
    segment = (
        base64.urlsafe_b64encode(json.dumps(payload).encode("utf-8")).rstrip(b"=").decode()
    )
    return f"placeholder-header.{segment}.placeholder-sig"


def test_decodes_email_when_payload_contains_urlsafe_chars():
    # Chosen so the base64url-encoded payload segment contains both `-` and
    # `_` — bytes that standard base64 (the old, buggy decoder) treats as
    # invalid and silently strips, corrupting the decode.
    token = _make_id_token({"email": "~yy?@example.com"})
    segment = token.split(".")[1]
    assert "-" in segment and "_" in segment  # sanity: this token exercises the bug

    assert _email_from_id_token({"id_token": token}) == "~yy?@example.com"


def test_decodes_email_when_segment_length_is_multiple_of_four():
    # Chosen so the base64url-encoded payload segment's length is already a
    # multiple of 4 (no padding needed) — the case the old `4 - len % 4`
    # formula got wrong (it appended a spurious "====").
    token = _make_id_token({"email": "userFz6Y@example.com"})
    segment = token.split(".")[1]
    assert len(segment) % 4 == 0  # sanity: this token exercises the bug

    assert _email_from_id_token({"id_token": token}) == "userFz6Y@example.com"


def test_returns_email_when_present():
    token = _make_id_token({"email": "person@example.com", "sub": "abc123"})
    assert _email_from_id_token({"id_token": token}) == "person@example.com"


def test_falls_back_to_sub_when_email_absent():
    token = _make_id_token({"sub": "abc123"})
    assert _email_from_id_token({"id_token": token}) == "abc123"


def test_returns_none_when_neither_email_nor_sub_present():
    token = _make_id_token({"aud": "some-client-id"})
    assert _email_from_id_token({"id_token": token}) is None


def test_returns_none_for_malformed_id_token():
    assert _email_from_id_token({"id_token": "not.a.jwt"}) is None


def test_returns_none_when_no_id_token_present():
    assert _email_from_id_token({}) is None


# Unit tests for `wso2_runner.oauth._state_matches`.
#
# Exercises the CSRF `state` comparison the login callback uses to decide
# whether a redirect back from Asgardeo is legitimate — purely through its
# external contract (expected state in, bool out), without spinning up the
# loopback HTTP server or a browser.


def test_state_matches_returns_true_for_matching_state():
    assert _state_matches("abc123", "abc123") is True


def test_state_matches_returns_false_for_different_state():
    assert _state_matches("abc123", "xyz789") is False


def test_state_matches_returns_false_when_returned_state_is_none():
    # The callback never received a `state` query parameter at all —
    # e.g. an attacker driving the browser straight to our redirect URI.
    assert _state_matches("abc123", None) is False


def test_state_matches_returns_false_for_empty_string_state():
    assert _state_matches("abc123", "") is False
