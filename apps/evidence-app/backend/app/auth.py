"""
User identity for the API.

Every request must carry `Authorization: Bearer <asgardeo_token>`. The token
is a JWT issued by Asgardeo; it is verified locally against Asgardeo's public
signing keys (JWKS) — there is no per-request call out to Asgardeo. Both the
web frontend and the local Runner authenticate this way (the Runner logs
into Asgardeo directly — see runner/wso2_runner/oauth.py).
"""
import logging

import jwt
from fastapi import HTTPException, Request, status
from jwt import PyJWKClient
from pydantic import BaseModel
from starlette.concurrency import run_in_threadpool

from app.config import settings

logger = logging.getLogger(__name__)

# Both derived from the single ASGARDEO_ORG setting, never from two
# independently-configured settings (see app/config.py). An issuer and a
# JWKS URL that could be pointed at different tenants would let a token from
# one tenant be verified with another tenant's signing keys — checking the
# issuer would then prove nothing.
_ASGARDEO_ISSUER = f"https://api.asgardeo.io/t/{settings.ASGARDEO_ORG}/oauth2/token"
_ASGARDEO_JWKS_URL = f"https://api.asgardeo.io/t/{settings.ASGARDEO_ORG}/oauth2/jwks"

# Built once, at import time — not inside get_current_user. PyJWKClient
# fetches Asgardeo's signing keys and caches them, which is what lets
# Asgardeo rotate its signing key without a redeploy here. Building this per
# request would throw that caching away and fetch keys on every single
# request — exactly the "network call on the request path" problem this
# module removes.
#
# The cache is NOT held until an unknown `kid` turns up: PyJWKClient defaults
# to `cache_jwk_set=True` with `lifespan=300`, so it also refetches on a
# five-minute timer. That matters because the fetch is a blocking urllib call
# with a 30s default timeout — see get_current_user, which keeps it off the
# event loop.
jwks = PyJWKClient(_ASGARDEO_JWKS_URL)


class User(BaseModel):
    email: str
    role: str  # "admin" | "engineer"


def _role_for(claims: dict) -> str:
    """Map an already signature-, issuer- and audience-verified token to a
    role in this application.

    Verified fact (checked against a real Asgardeo access token — see spec
    #71): the role claim is `roles`, a flat array of strings, and it is
    organisation-wide, NOT scoped to whichever client application the token
    was issued to. A token minted for the Runner carries the web app's role
    names too. That is why admin is gated on `aud` below rather than on a
    role name alone: `aud` is the only claim that says which application a
    token was actually issued to, so it's the only thing that can stop a
    Runner token from granting admin.

    Holding *some* role in the organisation is not enough to use this
    application. Asgardeo's default `everyone` role is held by every member
    of the organisation, so a bare valid token only proves its holder is
    someone in the org — not that they're meant to be in this app.
    Membership of the organisation must not imply membership of this
    application, so anyone without ASGARDEO_ENGINEER_ROLE (or, for admin,
    ASGARDEO_ADMIN_ROLE *and* the web app audience) is refused with a 403
    that says what to do about it, rather than silently downgraded to
    engineer.
    """
    roles = claims.get("roles", [])
    if isinstance(roles, str):
        roles = [roles]

    # `aud` is a plain string in every token observed so far, but Asgardeo
    # can emit a list. jwt.decode's own audience check already accepts
    # either shape when validating the token; this comparison has to as
    # well, or a single-element-array audience would fail the admin gate
    # even though the token is genuinely for the web app.
    aud = claims["aud"]
    auds = aud if isinstance(aud, list) else [aud]

    if settings.ASGARDEO_ADMIN_ROLE in roles and settings.ASGARDEO_WEBAPP_CLIENT_ID in auds:
        return "admin"
    if settings.ASGARDEO_ENGINEER_ROLE in roles:
        return "engineer"

    raise HTTPException(
        status_code=status.HTTP_403_FORBIDDEN,
        detail="You do not have access to the Evidence App. Ask an "
        "administrator to assign you the compliance evidence engineer "
        "role in Asgardeo.",
    )


async def get_current_user(request: Request) -> User:
    # Asgardeo Bearer token — verified locally against Asgardeo's public
    # signing keys (JWKS). Both the web frontend and the local Runner
    # authenticate this way.
    auth_header = request.headers.get("Authorization", "")
    if auth_header.startswith("Bearer "):
        token = auth_header[len("Bearer "):]

        try:
            # Off the event loop, deliberately. This looks like a cache read
            # and usually is, but PyJWKClient refetches whenever its cached
            # key set is older than `lifespan` (300s by default) as well as
            # when it meets an unknown `kid`, and that fetch is a blocking
            # urllib call with a 30s default timeout. Left inline in this
            # async dependency, a slow or unreachable Asgardeo would stall the
            # whole event loop for every concurrent request — including the
            # long-lived SSE task streams, which would simply stop updating
            # while a run was in progress.
            signing_key = await run_in_threadpool(jwks.get_signing_key_from_jwt, token)
            claims = jwt.decode(
                token,
                signing_key.key,
                algorithms=["RS256"],
                issuer=_ASGARDEO_ISSUER,
                audience=[settings.ASGARDEO_WEBAPP_CLIENT_ID, settings.ASGARDEO_RUNNER_CLIENT_ID],
            )
        except jwt.PyJWTError as exc:
            # Every rejection here — unknown signing key, bad signature,
            # expired, wrong issuer, audience outside our allow-list — is a
            # 401, never a 500: a bad token is an ordinary, expected outcome
            # of talking to the internet, not a server error. The reason is
            # logged for operators but never handed back to the caller, so a
            # rejected token can't be used to probe which specific check it
            # tripped.
            logger.warning("token rejected: %r", exc)
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Not authenticated. Please log in.",
            )

        # Never `sub` — that's a UUID, not an email. Read defensively rather
        # than indexing: Asgardeo leaves user attributes out of the access
        # token until they're explicitly added to it in the console (spec
        # #71, finding 4), so a tenant that hasn't been set up yet issues
        # genuine, correctly-signed tokens carrying no email at all. That is
        # a configuration mistake, not a server fault, and it must not
        # surface as a 500. Logged loudly because the fix is in the Asgardeo
        # console, not in this code, and nothing in the response says so.
        email = (claims.get("email") or "").strip()
        if not email:
            logger.warning(
                "token verified but carries no email claim — the email "
                "attribute is probably not added to the access token in "
                "the Asgardeo console (claims present: %s)",
                sorted(claims),
            )
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Not authenticated. Please log in.",
            )

        return User(email=email, role=_role_for(claims))

    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Not authenticated. Please log in.",
    )
