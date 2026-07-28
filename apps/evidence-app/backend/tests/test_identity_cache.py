"""
The identity cache (`app.auth._userinfo_cache`) exists so the backend
doesn't call Asgardeo's userinfo endpoint on every request — the Runner
alone polls every 2s. It honours a TTL on read, but historically never
removed anything: an entry sat in the dict for the life of the process even
after it expired, so a long-running service grew without bound as distinct
callers were seen over time (issue: identity cache stays bounded).

These tests drive the real `get_current_user` dependency (the `client`
fixture leaves it wired, unlike `engineer_client`/`admin_client`) over the
`/me` route, with Asgardeo's userinfo call faked so no network dependency
is needed and every call can be counted.

Cache *size* has no route exposing it, so there is no way to observe "an
expired entry stops occupying space" purely through HTTP responses; that
one assertion reaches into `app.auth._userinfo_cache` to read its length.
Everything else here is observed the same way the rest of the suite
prefers: through call counts and HTTP responses, not by asserting an
eviction function was called.
"""
import time

import pytest

import app.auth as auth_module


@pytest.fixture(autouse=True)
def _clear_identity_cache():
    """The cache is module-level state shared across the whole test run —
    without this, one test's cached identities would leak into the next
    and change its call counts."""
    auth_module._userinfo_cache.clear()
    yield
    auth_module._userinfo_cache.clear()


@pytest.fixture()
def fake_asgardeo(monkeypatch):
    """Replaces the Asgardeo userinfo call with an in-memory fake. Returns
    the list of bearer tokens actually sent to Asgardeo, in the order sent
    — the observable proxy for "did a resolution happen", so tests can
    assert on call counts instead of reaching into the cache to see
    whether a fetch occurred.
    """
    calls: list[str] = []

    class _FakeResponse:
        def __init__(self, token: str):
            self.status_code = 200
            self.text = "ok"
            self._token = token

        def json(self):
            return {"email": f"{self._token}@example.com"}

    class _FakeAsyncClient:
        def __init__(self, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc_info):
            return False

        async def get(self, url, headers=None):
            token = headers["Authorization"][len("Bearer "):]
            calls.append(token)
            return _FakeResponse(token)

    monkeypatch.setattr(auth_module.httpx, "AsyncClient", _FakeAsyncClient)
    return calls


def _whoami(client, token: str):
    return client.get("/api/me", headers={"Authorization": f"Bearer {token}"})


def test_fresh_caller_is_served_from_cache_without_recalling_asgardeo(client, fake_asgardeo):
    """Existing expiry behaviour, unchanged: a caller polled repeatedly
    within the TTL is resolved once and then served from cache — the whole
    reason the cache exists."""
    for _ in range(5):
        resp = _whoami(client, "tok-steady-poller")
        assert resp.status_code == 200

    assert len(fake_asgardeo) == 1


def test_expired_caller_is_re_resolved(client, fake_asgardeo, monkeypatch):
    """Existing expiry behaviour, unchanged: once TTL elapses, the same
    caller is re-resolved rather than served a stale cache entry."""
    monkeypatch.setattr(auth_module, "_USERINFO_CACHE_TTL_SECONDS", 0.05)

    assert _whoami(client, "tok-expires").status_code == 200
    assert len(fake_asgardeo) == 1

    time.sleep(0.1)

    assert _whoami(client, "tok-expires").status_code == 200
    assert len(fake_asgardeo) == 2


def test_distinct_fresh_callers_are_not_evicted_early(client, fake_asgardeo):
    """Two distinct callers seen close together, both still within TTL,
    both stay cached — the bounding behaviour must not evict entries that
    have not actually expired."""
    _whoami(client, "tok-a")
    _whoami(client, "tok-b")
    assert len(fake_asgardeo) == 2

    # Both still fresh: polling either again must not re-call Asgardeo.
    _whoami(client, "tok-a")
    _whoami(client, "tok-b")
    assert len(fake_asgardeo) == 2
    assert len(auth_module._userinfo_cache) == 2


def test_cache_stays_bounded_as_distinct_identities_are_seen_over_time(
    client, fake_asgardeo, monkeypatch
):
    """The core regression this ticket fixes: a long-running process that
    keeps seeing new distinct identities must not accumulate one cache
    entry per identity forever. Each caller here is only ever seen once,
    and is well past TTL by the time the next one arrives — exactly the
    "grows for the life of the process" scenario from the issue.

    Reaches into `_userinfo_cache` because there is no other way to
    observe that space was actually reclaimed, not just that expiry was
    honoured on the next read of the *same* key.
    """
    monkeypatch.setattr(auth_module, "_USERINFO_CACHE_TTL_SECONDS", 0.02)

    for i in range(30):
        assert _whoami(client, f"tok-once-{i}").status_code == 200
        time.sleep(0.03)  # each entry is expired well before the next call

    assert len(fake_asgardeo) == 30
    # Unbounded (pre-fix) behaviour would leave all 30 entries sitting in
    # the dict. Bounded behaviour leaves only the most recent one or two
    # (the last one written, plus possibly one not yet swept).
    assert len(auth_module._userinfo_cache) <= 2
