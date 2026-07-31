"""
Real JWT verification in `get_current_user` (app/auth.py), exercised end to
end over HTTP against the real dependency -- the `client` fixture leaves
`get_current_user` wired, unlike `engineer_client` / `admin_client`, which
fake it entirely.

Every rejection case here is the whole point of the JWKS rewrite (spec #71):
the old userinfo-based check only ever asked Asgardeo "is this a token you
issued", so overriding `get_current_user` (as most of the suite does, and
still should) would bypass verification entirely and couldn't exercise a
single one of these cases.

Instead, `app.auth.jwks` -- the module-level `PyJWKClient` -- is swapped for
a fake exposing the one method `get_current_user` calls,
`get_signing_key_from_jwt`, which always hands back the public half of a key
pair generated in this file. Tokens are then signed against the matching
private key with PyJWT, exactly as Asgardeo would sign a real one, so the
suite exercises real `jwt.decode` validation (signature, issuer, audience,
expiry) without ever touching the network. The issuer needs no seam of its
own: it derives from `ASGARDEO_ORG`, which conftest already sets to
"test-org". This is the same class of decision as the blob-storage test seam
in conftest.py's `blob_container` -- settled up front rather than discovered
mid-test.

Never put a real Asgardeo token here. Every token below is signed locally
against a key generated in this process and thrown away when the run ends.
"""
import time

import jwt
import pytest
from cryptography.hazmat.primitives.asymmetric import rsa

import app.auth as auth_module
from app.config import settings

# One key pair "Asgardeo" is trusted to have signed with, and one it never
# touched -- generated once for the whole module, since RSA keygen is slow
# and every test here only needs *a* key pair, not a fresh one.
_TRUSTED_KEY = rsa.generate_private_key(public_exponent=65537, key_size=2048)
_WRONG_KEY = rsa.generate_private_key(public_exponent=65537, key_size=2048)


class _FakeSigningKey:
    """Stands in for `jwt.api_jwk.PyJWK`, which is what a real
    `PyJWKClient.get_signing_key_from_jwt` returns. `get_current_user` only
    ever reads its `.key` attribute, so that's the only thing this needs to
    have."""

    def __init__(self, key):
        self.key = key


class _FakeJWKS:
    """Replaces `app.auth.jwks` for the duration of a test. Always hands
    back the trusted public key, regardless of the token's `kid` -- these
    tests exercise what `get_current_user` does with whatever key it's
    given, not `PyJWKClient`'s own key-rotation lookup."""

    def get_signing_key_from_jwt(self, token):
        return _FakeSigningKey(_TRUSTED_KEY.public_key())


@pytest.fixture(autouse=True)
def _use_fake_jwks(monkeypatch):
    monkeypatch.setattr(auth_module, "jwks", _FakeJWKS())


def _make_token(
    *,
    key=None,
    email="engineer@example.com",
    sub="11111111-1111-1111-1111-111111111111",
    roles=(settings.ASGARDEO_ENGINEER_ROLE,),
    aud=None,
    issuer=None,
    expires_in=3600,
) -> str:
    """Builds and signs a token shaped like a real Asgardeo access token
    (see spec #71's decoded example) with just enough claims for
    `get_current_user` to act on. Every parameter defaults to a value that
    passes every check, so each test below only has to override the one
    thing it's testing."""
    now = int(time.time())
    claims = {
        "sub": sub,
        "roles": list(roles),
        "iss": issuer if issuer is not None else auth_module._ASGARDEO_ISSUER,
        "aud": aud if aud is not None else settings.ASGARDEO_WEBAPP_CLIENT_ID,
        "iat": now,
        "nbf": now,
        "exp": now + expires_in,
    }
    # `email=None` omits the claim entirely rather than sending a null,
    # because that's the shape Asgardeo actually issues before the email
    # attribute is added to the access token — the claim is simply absent.
    if email is not None:
        claims["email"] = email
    return jwt.encode(claims, key or _TRUSTED_KEY, algorithm="RS256")


def _whoami(client, token: str):
    return client.get("/api/me", headers={"Authorization": f"Bearer {token}"})


# --- Acceptance -------------------------------------------------------


def test_valid_webapp_token_with_admin_role_resolves_to_admin(client):
    token = _make_token(
        roles=[settings.ASGARDEO_ADMIN_ROLE],
        aud=settings.ASGARDEO_WEBAPP_CLIENT_ID,
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["role"] == "admin"


def test_valid_webapp_token_with_engineer_role_resolves_to_engineer(client):
    token = _make_token(
        roles=[settings.ASGARDEO_ENGINEER_ROLE],
        aud=settings.ASGARDEO_WEBAPP_CLIENT_ID,
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["role"] == "engineer"


def test_valid_runner_token_with_engineer_role_resolves_to_engineer(client):
    token = _make_token(
        roles=[settings.ASGARDEO_ENGINEER_ROLE],
        aud=settings.ASGARDEO_RUNNER_CLIENT_ID,
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["role"] == "engineer"


def test_identity_comes_from_email_claim_not_sub(client):
    """`sub` is a UUID, not an email (spec #71). If `get_current_user` ever
    read `sub` instead of `email`, this would surface the UUID as the
    user's identity instead of their address."""
    token = _make_token(
        email="someone@example.com",
        sub="99999999-9999-9999-9999-999999999999",
        roles=[settings.ASGARDEO_ENGINEER_ROLE],
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["email"] == "someone@example.com"


# --- Rejection ----------------------------------------------------------


def test_token_signed_by_wrong_key_is_rejected(client):
    token = _make_token(key=_WRONG_KEY)
    assert _whoami(client, token).status_code == 401


def test_expired_token_is_rejected(client):
    token = _make_token(expires_in=-60)
    assert _whoami(client, token).status_code == 401


def test_wrong_issuer_is_rejected(client):
    token = _make_token(issuer="https://api.asgardeo.io/t/some-other-org/oauth2/token")
    assert _whoami(client, token).status_code == 401


def test_audience_outside_allow_list_is_rejected(client):
    """A token that is otherwise perfectly valid -- signed by the trusted
    key, unexpired, correct issuer -- but minted for some unrelated
    Asgardeo application must still be rejected. This is the case this
    deployment specifically needs: two client applications share one
    Asgardeo organisation, and only tokens minted for one of *our* two
    clients may be accepted (spec #71)."""
    token = _make_token(aud="some-unrelated-application-client-id")
    assert _whoami(client, token).status_code == 401


def test_in_audience_token_with_no_role_is_rejected(client):
    """Spec #71, finding 7: a genuine, in-audience token is not enough on
    its own -- the holder must also carry a role in *this* application.
    Without this check, any member of the Asgardeo organisation who can
    obtain a token becomes an engineer, with no assignment at all."""
    token = _make_token(roles=[])
    assert _whoami(client, token).status_code == 403


def test_admin_role_with_runner_audience_resolves_to_engineer_not_admin(client):
    """Decision 4: roles are organisation-wide, not per-application, so an
    admin (who is also assigned the engineer role, same as the real
    deployment's own admins would be -- see spec #71's decoded example,
    where a Runner-issued token carries both an admin *and* the
    org-assigned role list together) gets a Runner-issued token carrying
    the admin role name too. `aud` is the only claim that can stop it
    granting admin -- this is the test that proves privilege separation
    actually works: the same holder, same roles, only the audience
    differs, and that alone must be what downgrades them to engineer."""
    token = _make_token(
        roles=[settings.ASGARDEO_ADMIN_ROLE, settings.ASGARDEO_ENGINEER_ROLE],
        aud=settings.ASGARDEO_RUNNER_CLIENT_ID,
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["role"] == "engineer"


def test_token_with_no_email_claim_is_rejected_not_a_server_error(client):
    """A correctly signed, in-audience, unexpired token that carries no
    `email` claim is what an Asgardeo tenant issues before the email
    attribute has been added to the access token (spec #71, finding 4).

    The holder can't be identified, so the request can't proceed — but this
    is a configuration mistake, and it must come back as a 401 rather than
    a 500. Every user in a freshly configured tenant hits this path at
    once, and "the server is broken" sends people hunting in the wrong
    place entirely."""
    token = _make_token(email=None)
    assert _whoami(client, token).status_code == 401


def test_everyone_role_alone_is_rejected(client):
    """`everyone` is Asgardeo's default role, held by every member of the
    organisation -- it must never satisfy an authorisation check on its
    own."""
    token = _make_token(roles=["everyone"])
    assert _whoami(client, token).status_code == 403


def test_audience_as_single_element_array_is_accepted(client):
    """`aud` is a plain string in every token observed so far, but Asgardeo
    can emit a list (spec #71). The admin gate's own `aud` comparison has to
    accept that shape too, not just `jwt.decode`'s own audience
    validation."""
    token = _make_token(
        roles=[settings.ASGARDEO_ADMIN_ROLE],
        aud=[settings.ASGARDEO_WEBAPP_CLIENT_ID],
    )
    resp = _whoami(client, token)
    assert resp.status_code == 200
    assert resp.json()["role"] == "admin"
