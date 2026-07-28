"""
User identity for the API.

Every request must carry `Authorization: Bearer <asgardeo_token>`. The token is
validated against Asgardeo's UserInfo endpoint. Both the web frontend and the
local Runner authenticate this way (the Runner logs into Asgardeo directly —
see runner/wso2_runner/oauth.py).
"""
import hashlib
import logging
import time

import httpx
from fastapi import HTTPException, Request, status
from pydantic import BaseModel

from app.config import settings

logger = logging.getLogger(__name__)

_USERINFO_URL = f"https://api.asgardeo.io/t/{settings.ASGARDEO_ORG}/oauth2/userinfo"

# Short-lived cache so we don't call Asgardeo's userinfo endpoint on every
# single request (the Runner alone polls every 2s). Keyed by a hash of the
# token, not the raw token, so a memory dump doesn't hand out live bearer
# tokens. A short TTL keeps this from meaningfully delaying role/permission
# changes while cutting call volume ~15x during steady polling.
_USERINFO_CACHE_TTL_SECONDS = 30
_userinfo_cache: dict[str, tuple[dict, float]] = {}


def _cache_key(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


def _evict_expired_cache_entries(now: float) -> None:
    """Remove every cache entry whose TTL has elapsed.

    Called at the top of every request, before the cache is even looked
    up, so an entry stops occupying space the moment it's stale — not just
    when a read happens to land on that exact key. Without this, the only
    thing that ever removed an entry was the same caller polling again
    after it expired; a caller who stops polling (token rotated, session
    ended) left their entry behind for the life of the process. The Runner
    alone polls every 2s, so a long-running service grew steadily.

    A plain dict scan, run every request: cheap relative to the network
    call it's guarding, and the dict it scans is itself bounded by this
    same eviction, so it never has room to become expensive.
    """
    expired = [
        key
        for key, (_, cached_at) in _userinfo_cache.items()
        if now - cached_at >= _USERINFO_CACHE_TTL_SECONDS
    ]
    for key in expired:
        del _userinfo_cache[key]


class User(BaseModel):
    email: str
    role: str  # "admin" | "engineer"


def _role_for(email: str, claims: dict | None = None) -> str:
    """Admin if an Asgardeo role/group claim says so, else if the email is in
    the ADMIN_EMAILS allow-list, else engineer.

    `claims` is the Asgardeo userinfo response for this user. Asgardeo isn't
    configured with application roles/groups yet; once it is, whichever of
    "roles" / "groups" / "role" it populates will be picked up here
    automatically with no further code change. Verify the actual claim name
    against a real Asgardeo userinfo response once that console setup is done,
    and adjust the keys checked below if it differs. ADMIN_EMAILS can be retired
    once the Asgardeo-side claim is confirmed working for every admin.
    """
    if claims:
        raw = claims.get("roles") or claims.get("groups") or claims.get("role") or []
        if isinstance(raw, str):
            raw = [raw]
        if any(str(r).strip().lower() == "admin" for r in raw):
            return "admin"

    admin_emails = {e.strip().lower() for e in settings.ADMIN_EMAILS.split(",") if e.strip()}
    return "admin" if email.strip().lower() in admin_emails else "engineer"


async def get_current_user(request: Request) -> User:
    # Asgardeo Bearer token — validated against Asgardeo's UserInfo endpoint.
    # Both the web frontend and the local Runner authenticate this way.
    auth_header = request.headers.get("Authorization", "")
    if auth_header.startswith("Bearer "):
        token = auth_header[len("Bearer "):]
        key = _cache_key(token)
        now = time.monotonic()
        _evict_expired_cache_entries(now)
        cached = _userinfo_cache.get(key)

        if cached and now - cached[1] < _USERINFO_CACHE_TTL_SECONDS:
            info = cached[0]
        else:
            try:
                async with httpx.AsyncClient(timeout=10.0) as client:
                    resp = await client.get(
                        _USERINFO_URL,
                        headers={"Authorization": f"Bearer {token}"},
                    )
            except httpx.HTTPError as exc:
                logger.warning("userinfo call errored: %r", exc)
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Could not reach Asgardeo to validate this token — try again.",
                )

            if resp.status_code != 200:
                logger.warning(
                    "userinfo call failed: HTTP %s — %s",
                    resp.status_code,
                    resp.text[:500],
                )
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Not authenticated. Please log in.",
                )

            info = resp.json()
            _userinfo_cache[key] = (info, now)

        email = (info.get("email") or info.get("sub") or "").strip()
        if email:
            return User(email=email, role=_role_for(email, claims=info))
        logger.warning("userinfo 200 but no email/sub in response: %r", info)

    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Not authenticated. Please log in.",
    )
