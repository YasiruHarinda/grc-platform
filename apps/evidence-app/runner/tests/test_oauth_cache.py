"""Unit tests for the token-cache and access-token logic in `wso2_runner.oauth`.

Covers the pieces `tests/test_oauth.py` doesn't touch:

  * `_pkce_pair` — the PKCE verifier/challenge generation.
  * `_save_cache` / `_load_cache` — the on-disk JSON round trip, isolated
    from the real `~/.wso2-runner/` cache by monkeypatching
    `TOKEN_CACHE_FILE` to a path under `tmp_path`.
  * `has_cached_session` / `current_email` — thin wrappers over the cache.
  * `get_access_token` — the orchestration of cache-hit / refresh /
    interactive-login, with `_refresh` and `_login_interactive` replaced by
    call-recording fakes so no network call or browser ever happens.
"""
import base64
import hashlib
import json
import time

import pytest

import wso2_runner.oauth as oauth


def _make_id_token(payload: dict) -> str:
    """Build a `header.payload.sig` string shaped like a real JWT,
    base64url-encoding only the payload segment (the only one the module
    reads). Header and signature are placeholders."""
    segment = (
        base64.urlsafe_b64encode(json.dumps(payload).encode("utf-8")).rstrip(b"=").decode()
    )
    return f"placeholder-header.{segment}.placeholder-sig"


def _token(email: str = "person@example.com", **overrides) -> dict:
    """A cache-shaped token dict with sane defaults, overridable per test."""
    tokens = {
        "access_token": "access-token-value",
        "refresh_token": "refresh-token-value",
        "id_token": _make_id_token({"email": email}),
        "expires_in": 3600,
        "_obtained_at": time.time(),
    }
    tokens.update(overrides)
    return tokens


@pytest.fixture(autouse=True)
def isolated_cache_file(tmp_path, monkeypatch):
    """Point the module's cache file at a tmp path so no test ever touches
    the real ~/.wso2-runner/token_cache.json."""
    cache_file = tmp_path / "token_cache.json"
    monkeypatch.setattr(oauth, "TOKEN_CACHE_FILE", cache_file)
    return cache_file


# ---------------------------------------------------------------------------
# _pkce_pair
# ---------------------------------------------------------------------------


def test_pkce_pair_returns_urlsafe_unpadded_verifier_and_challenge():
    verifier, challenge = oauth._pkce_pair()

    assert "=" not in verifier
    assert "=" not in challenge
    # base64url alphabet only: letters, digits, '-', '_'
    assert all(c.isalnum() or c in "-_" for c in verifier)
    assert all(c.isalnum() or c in "-_" for c in challenge)


def test_pkce_pair_challenge_is_sha256_of_verifier():
    verifier, challenge = oauth._pkce_pair()

    expected = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
    assert challenge == expected


def test_pkce_pair_generates_distinct_pairs_each_call():
    v1, _ = oauth._pkce_pair()
    v2, _ = oauth._pkce_pair()
    assert v1 != v2


# ---------------------------------------------------------------------------
# _save_cache / _load_cache
# ---------------------------------------------------------------------------


def test_save_then_load_round_trips_the_same_dict():
    tokens = _token()
    oauth._save_cache(tokens)

    assert oauth._load_cache() == tokens


def test_load_cache_returns_none_when_file_missing():
    assert oauth._load_cache() is None


def test_load_cache_returns_none_for_corrupt_json(isolated_cache_file):
    isolated_cache_file.parent.mkdir(parents=True, exist_ok=True)
    isolated_cache_file.write_text("{not valid json")

    assert oauth._load_cache() is None


# ---------------------------------------------------------------------------
# has_cached_session
# ---------------------------------------------------------------------------


def test_has_cached_session_true_when_cache_exists():
    oauth._save_cache(_token())
    assert oauth.has_cached_session() is True


def test_has_cached_session_false_when_no_cache():
    assert oauth.has_cached_session() is False


# ---------------------------------------------------------------------------
# current_email
# ---------------------------------------------------------------------------


def test_current_email_returns_email_from_cached_id_token():
    oauth._save_cache(_token(email="cached@example.com"))
    assert oauth.current_email() == "cached@example.com"


def test_current_email_returns_none_when_no_cache():
    assert oauth.current_email() is None


# ---------------------------------------------------------------------------
# get_access_token
# ---------------------------------------------------------------------------


class _FakeCallRecorder:
    """A call-recording fake for `_refresh`/`_login_interactive`: records
    every call's args and returns a preset token dict (or raises)."""

    def __init__(self, result=None, raises=None):
        self.result = result
        self.raises = raises
        self.calls = []

    def __call__(self, *args, **kwargs):
        self.calls.append((args, kwargs))
        if self.raises is not None:
            raise self.raises
        return self.result


def test_returns_cached_access_token_without_refresh_or_login_when_still_valid(monkeypatch):
    cached = _token(access_token="still-valid-token", _obtained_at=time.time(), expires_in=3600)
    oauth._save_cache(cached)

    fake_refresh = _FakeCallRecorder()
    fake_login = _FakeCallRecorder()
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id")

    assert result == "still-valid-token"
    assert fake_refresh.calls == []
    assert fake_login.calls == []


def test_expired_cache_with_refresh_token_triggers_refresh_not_login(monkeypatch):
    expired = _token(
        access_token="stale-token",
        refresh_token="my-refresh-token",
        _obtained_at=time.time() - 7200,
        expires_in=3600,
    )
    oauth._save_cache(expired)

    new_tokens = _token(access_token="refreshed-token", _obtained_at=time.time())
    fake_refresh = _FakeCallRecorder(result=new_tokens)
    fake_login = _FakeCallRecorder()
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id")

    assert result == "refreshed-token"
    assert len(fake_refresh.calls) == 1
    assert fake_refresh.calls[0][0] == ("myorg", "client-id", "my-refresh-token")
    assert fake_login.calls == []
    assert oauth._load_cache()["access_token"] == "refreshed-token"


def test_refresh_failure_falls_back_to_interactive_login(monkeypatch):
    expired = _token(
        access_token="stale-token",
        refresh_token="my-refresh-token",
        _obtained_at=time.time() - 7200,
        expires_in=3600,
    )
    oauth._save_cache(expired)

    login_tokens = _token(access_token="fresh-login-token", _obtained_at=time.time())
    fake_refresh = _FakeCallRecorder(raises=RuntimeError("refresh token expired"))
    fake_login = _FakeCallRecorder(result=login_tokens)
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id")

    assert result == "fresh-login-token"
    assert len(fake_refresh.calls) == 1
    assert len(fake_login.calls) == 1
    assert oauth._load_cache()["access_token"] == "fresh-login-token"


def test_login_hint_mismatch_forces_fresh_login_even_with_valid_cache(monkeypatch):
    cached = _token(
        email="alice@example.com",
        access_token="alices-valid-token",
        _obtained_at=time.time(),
        expires_in=3600,
    )
    oauth._save_cache(cached)

    login_tokens = _token(email="bob@example.com", access_token="bobs-token", _obtained_at=time.time())
    fake_refresh = _FakeCallRecorder()
    fake_login = _FakeCallRecorder(result=login_tokens)
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id", login_hint="bob@example.com")

    assert result == "bobs-token"
    assert fake_refresh.calls == []
    assert len(fake_login.calls) == 1
    assert oauth._load_cache()["access_token"] == "bobs-token"


def test_force_login_ignores_valid_cache_and_calls_interactive_login(monkeypatch):
    cached = _token(access_token="still-valid-token", _obtained_at=time.time(), expires_in=3600)
    oauth._save_cache(cached)

    login_tokens = _token(access_token="forced-login-token", _obtained_at=time.time())
    fake_refresh = _FakeCallRecorder()
    fake_login = _FakeCallRecorder(result=login_tokens)
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id", force_login=True)

    assert result == "forced-login-token"
    assert fake_refresh.calls == []
    assert len(fake_login.calls) == 1
    assert oauth._load_cache()["access_token"] == "forced-login-token"


def test_no_cache_calls_interactive_login_and_returns_saved_token(monkeypatch):
    login_tokens = _token(access_token="brand-new-token", _obtained_at=time.time())
    fake_refresh = _FakeCallRecorder()
    fake_login = _FakeCallRecorder(result=login_tokens)
    monkeypatch.setattr(oauth, "_refresh", fake_refresh)
    monkeypatch.setattr(oauth, "_login_interactive", fake_login)

    result = oauth.get_access_token("myorg", "client-id", login_hint="someone@example.com")

    assert result == "brand-new-token"
    assert fake_refresh.calls == []
    assert len(fake_login.calls) == 1
    assert fake_login.calls[0][0] == ("myorg", "client-id", "someone@example.com")
    assert oauth._load_cache()["access_token"] == "brand-new-token"
