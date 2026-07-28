"""Asgardeo login for the local Runner.

Uses the OAuth 2.0 Authorization Code flow with PKCE and a local loopback
listener — the same pattern CLI tools like `gcloud auth login`, `aws sso
login`, and `az login` use: a browser window opens for the real Asgardeo
login page (including MFA), and once signed in, the browser redirects to a
tiny local web server this module starts, which captures the result.

Tokens are cached in ~/.wso2-runner/token_cache.json and silently refreshed
in the background (see get_access_token). A fresh interactive login is only
needed again once the refresh token itself expires (about once a day) —
identical to how the web frontend's Asgardeo session behaves.
"""

import base64
import hashlib
import http.server
import json
import secrets
import threading
import time
import urllib.parse
import webbrowser
from pathlib import Path

import httpx

TOKEN_CACHE_FILE = Path.home() / ".wso2-runner" / "token_cache.json"

# Must match a redirect URI registered on the Runner's Asgardeo application
# (a "Native Application" / public client, separate from the web frontend's
# registration — see the setup steps for creating it).
CALLBACK_PORT = 8765
CALLBACK_PATH = "/callback"

SCOPE = "openid email profile groups"

# How much earlier than the real expiry we treat a token as "needs refresh",
# so a request never fires with a token that expires mid-flight.
_EXPIRY_SAFETY_MARGIN_SECONDS = 60


class _CallbackHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:  # noqa: N802 (stdlib method name)
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path != CALLBACK_PATH:
            self.send_response(404)
            self.end_headers()
            return

        params = urllib.parse.parse_qs(parsed.query)
        self.server.auth_code = params.get("code", [None])[0]  # type: ignore[attr-defined]
        self.server.auth_error = params.get(  # type: ignore[attr-defined]
            "error_description", params.get("error", [None])
        )[0]
        self.server.auth_state = params.get("state", [None])[0]  # type: ignore[attr-defined]

        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        if self.server.auth_code:  # type: ignore[attr-defined]
            self.wfile.write(
                b"<html><body style='font-family:sans-serif'>"
                b"<h2>Signed in</h2><p>You can close this tab and return to the terminal.</p>"
                b"</body></html>"
            )
        else:
            self.wfile.write(
                b"<html><body style='font-family:sans-serif'>"
                b"<h2>Sign-in failed</h2><p>Return to the terminal and try again.</p>"
                b"</body></html>"
            )

    def log_message(self, format: str, *args) -> None:  # noqa: A002
        pass  # silence default per-request stderr logging


def _state_matches(expected: str, returned: str | None) -> bool:
    """True only when the callback actually echoed back our CSRF `state`."""
    return returned is not None and returned != "" and returned == expected


def _pkce_pair() -> tuple[str, str]:
    verifier = base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b"=").decode()
    challenge = (
        base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
    )
    return verifier, challenge


def _load_cache() -> dict | None:
    if not TOKEN_CACHE_FILE.exists():
        return None
    try:
        return json.loads(TOKEN_CACHE_FILE.read_text())
    except Exception:
        return None


def _save_cache(tokens: dict) -> None:
    TOKEN_CACHE_FILE.parent.mkdir(parents=True, exist_ok=True)
    TOKEN_CACHE_FILE.write_text(json.dumps(tokens))
    try:
        TOKEN_CACHE_FILE.chmod(0o600)  # best-effort: owner-only, like an SSH key
    except Exception:
        pass


def _login_interactive(org: str, client_id: str, login_hint: str | None) -> dict:
    """Open the browser for a fresh Asgardeo login. Returns the token response."""
    verifier, challenge = _pkce_pair()
    state = secrets.token_urlsafe(16)
    redirect_uri = f"http://localhost:{CALLBACK_PORT}{CALLBACK_PATH}"

    params = {
        "response_type": "code",
        "client_id": client_id,
        "redirect_uri": redirect_uri,
        "scope": SCOPE,
        "state": state,
        "code_challenge": challenge,
        "code_challenge_method": "S256",
    }
    if login_hint:
        params["login_hint"] = login_hint

    authorize_url = f"https://api.asgardeo.io/t/{org}/oauth2/authorize?" + urllib.parse.urlencode(params)

    server = http.server.HTTPServer(("localhost", CALLBACK_PORT), _CallbackHandler)
    server.auth_code = None  # type: ignore[attr-defined]
    server.auth_error = None  # type: ignore[attr-defined]
    server.auth_state = None  # type: ignore[attr-defined]
    thread = threading.Thread(target=server.handle_request, daemon=True)
    thread.start()

    print("[runner] Opening browser for Asgardeo login...")
    print("[runner] If it doesn't open automatically, visit:")
    print(f"[runner]   {authorize_url}\n")
    webbrowser.open(authorize_url)

    thread.join(timeout=180)
    server.server_close()

    if server.auth_error:  # type: ignore[attr-defined]
        raise RuntimeError(f"Asgardeo login failed: {server.auth_error}")  # type: ignore[attr-defined]
    if not server.auth_code:  # type: ignore[attr-defined]
        raise RuntimeError(
            "Timed out waiting for Asgardeo login (180s). Run the command again."
        )
    if not _state_matches(state, server.auth_state):  # type: ignore[attr-defined]
        raise RuntimeError("Asgardeo login failed: state mismatch (possible CSRF attempt).")

    resp = httpx.post(
        f"https://api.asgardeo.io/t/{org}/oauth2/token",
        data={
            "grant_type": "authorization_code",
            "client_id": client_id,
            "code": server.auth_code,  # type: ignore[attr-defined]
            "redirect_uri": redirect_uri,
            "code_verifier": verifier,
        },
    )
    resp.raise_for_status()
    tokens = resp.json()
    tokens["_obtained_at"] = time.time()
    return tokens


def _refresh(org: str, client_id: str, refresh_token: str) -> dict:
    resp = httpx.post(
        f"https://api.asgardeo.io/t/{org}/oauth2/token",
        data={
            "grant_type": "refresh_token",
            "client_id": client_id,
            "refresh_token": refresh_token,
        },
    )
    resp.raise_for_status()
    tokens = resp.json()
    tokens["_obtained_at"] = time.time()
    return tokens


def get_access_token(
    org: str,
    client_id: str,
    login_hint: str | None = None,
    force_login: bool = False,
) -> str:
    """Return a currently-valid access token, refreshing or logging in as needed.

    Cheap in the common case: if the cached access token is still valid, this
    just reads a small local file — no network call. Only hits the network
    for the hourly silent refresh, or for a fresh interactive login when the
    refresh token itself has expired (roughly once a day).
    """
    cached = None if force_login else _load_cache()

    # If the caller explicitly asked for a different account than whoever is
    # actually cached, treat that as "switch accounts" rather than silently
    # keeping the cached session logged in as someone else.
    if cached and login_hint:
        cached_email = _email_from_id_token(cached)
        if cached_email and cached_email.strip().lower() != login_hint.strip().lower():
            print(f"[runner] Cached session is {cached_email}, not {login_hint} — signing in again...")
            cached = None

    if cached:
        age = time.time() - cached.get("_obtained_at", 0)
        if age < cached.get("expires_in", 3600) - _EXPIRY_SAFETY_MARGIN_SECONDS:
            return cached["access_token"]

        refresh_token = cached.get("refresh_token")
        if refresh_token:
            try:
                tokens = _refresh(org, client_id, refresh_token)
                tokens.setdefault("refresh_token", refresh_token)
                _save_cache(tokens)
                return tokens["access_token"]
            except Exception as e:
                print(f"[runner] token refresh failed ({e!r}); falling back to interactive login")
                pass  # refresh token expired/invalid — fall through to interactive login

    tokens = _login_interactive(org, client_id, login_hint)
    _save_cache(tokens)
    return tokens["access_token"]


def has_cached_session() -> bool:
    """True if a prior login (even one due for silent refresh) is cached locally."""
    return _load_cache() is not None


def _email_from_id_token(tokens: dict) -> str | None:
    id_token = tokens.get("id_token")
    if not id_token:
        return None
    try:
        parts = id_token.split(".")
        padding = -len(parts[1]) % 4
        payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=" * padding).decode("utf-8"))
        return payload.get("email") or payload.get("sub")
    except Exception:
        return None


def current_email() -> str | None:
    """Best-effort email of whoever is currently cached as logged in, for display only."""
    cached = _load_cache()
    return _email_from_id_token(cached) if cached else None
