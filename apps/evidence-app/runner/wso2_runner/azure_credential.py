"""Azure Entra credential for Azure OpenAI calls.

Kept as its own module, separate from `wso2_runner.agent`, so it can be
imported — and tested — without pulling in `browser_use` or anything else
that reaches a browser. Its only job is producing an Azure credential and a
token provider.

`AZURE_OPENAI_AUTH_MODE` (default `"entra"`) picks how the Runner
authorises its Azure OpenAI calls:

- `"entra"` — each engineer's own Azure CLI session (`az login`) authorises
  the call, scoped to Azure Cognitive Services. No credential is ever
  written to disk by the Runner.
- `"api_key"` — the long-lived `AZURE_OPENAI_API_KEY`, kept only for the
  rollout period. Reachable *only* by choosing this mode explicitly — a key
  merely being present in config never activates it on its own.

The credential is built once, at module scope, and reused for the life of
the process (see `_credential` / `_token_provider` below). `_build_llm()`
runs once per task, so a per-call credential would shell out to the Azure
CLI — and rebuild its token cache — on every single task. That would be
correct but slow and wasteful; sharing one credential is deliberate and
should not be "simplified" into a local.
"""

from typing import Awaitable, Callable

import httpx
from azure.core.exceptions import ClientAuthenticationError  # noqa: F401 (re-exported for callers)
from azure.identity import CredentialUnavailableError  # noqa: F401 (re-exported for callers)
from azure.identity.aio import AzureCliCredential, get_bearer_token_provider

from wso2_runner.config import AZURE_AUTH_API_KEY, settings

# Resource scope for Azure OpenAI / Azure Cognitive Services data-plane calls.
_COGNITIVE_SERVICES_SCOPE = "https://cognitiveservices.azure.com/.default"

TokenProvider = Callable[[], Awaitable[str]]

# How long the access probe below waits. Short: it runs in front of every
# `start`, and a slow answer must not become a slow launch.
_PROBE_TIMEOUT_SECONDS = 10.0


class AzureAccessDeniedError(Exception):
    """Authentication succeeded, but the signed-in identity is not
    authorised to call Azure OpenAI.

    This is its own type, and not one of the vendor's, because the vendor
    library can never raise it: Azure AD issues an access token to any
    authenticated member of the tenant, and the Cognitive Services role is
    checked by the *resource* when the token is presented, not by AD when it
    is issued. Only a real call to the endpoint can tell the difference, so
    only `verify_access()` below can raise this.
    """


class AzureAccessUnverifiedError(Exception):
    """The access probe could not reach a verdict — no endpoint is
    configured, the endpoint could not be reached, or it answered something
    other than success or a refusal.

    Deliberately distinct from `AzureAccessDeniedError`: a laptop on bad
    wifi must not be told to go and ask an administrator for a role it
    already has. Callers treat this as a warning, never as a failure.
    """

# Built once, at import time of first use, and reused for the rest of the
# process so the credential's in-memory token cache is shared across every
# task instead of being rebuilt (and re-authenticating with the Azure CLI)
# per task. Do not move either of these into a function-local.
_credential: AzureCliCredential | None = None
_token_provider: TokenProvider | None = None


def _get_credential() -> AzureCliCredential:
    """Build (once) and return the shared Azure CLI credential.

    Deliberately constructed *with* `AZURE_TENANT_ID`: engineers switch
    Azure tenants routinely while collecting evidence, and an unpinned
    credential would silently follow whichever tenant the CLI last
    selected, breaking LLM access with a confusing error.
    """
    global _credential
    if _credential is None:
        tenant_id = settings.AZURE_TENANT_ID
        if not tenant_id:
            raise RuntimeError(
                "AZURE_OPENAI_AUTH_MODE is 'entra' but AZURE_TENANT_ID is not "
                "set. Set AZURE_TENANT_ID in your Runner config (see the "
                "setup docs), or set AZURE_OPENAI_AUTH_MODE=api_key if you "
                "deliberately want to keep using an API key."
            )
        _credential = AzureCliCredential(tenant_id=tenant_id)
    return _credential


def _get_token_provider() -> TokenProvider:
    """Build (once) and return the shared async bearer-token provider."""
    global _token_provider
    if _token_provider is None:
        _token_provider = get_bearer_token_provider(_get_credential(), _COGNITIVE_SERVICES_SCOPE)
    return _token_provider


def resolve_llm_auth() -> TokenProvider | None:
    """What `_build_llm()` (ticket #94) passes to `ChatAzureOpenAI`.

    Returns the shared async bearer-token provider — pass it as
    `azure_ad_token_provider` — when `AZURE_OPENAI_AUTH_MODE` is `"entra"`,
    the default. Returns `None` when the mode is explicitly `"api_key"`:
    the signal for the caller to fall back to `AZURE_OPENAI_API_KEY` as
    `api_key` instead.

    The mode is read verbatim from settings — an `AZURE_OPENAI_API_KEY`
    that merely happens to be present never switches the mode on its own.
    There is no implicit fallback in either direction.
    """
    if settings.AZURE_OPENAI_AUTH_MODE == AZURE_AUTH_API_KEY:
        return None
    return _get_token_provider()


async def attempt_token() -> str:
    """Obtain one real Azure AD access token, for `wso2-runner doctor`
    (ticket #95) to call.

    Returns the token string on success. On failure, the vendor library's
    own exceptions propagate unmodified so the caller can tell the failure
    modes apart *by type* — never by matching an error message:

    - `CredentialUnavailableError` — the credential could not even attempt
      authentication: the Azure CLI isn't installed, or nobody is signed
      in.
    - `ClientAuthenticationError` — authentication was attempted and
      rejected: e.g. signed in to the wrong tenant, so the Azure CLI cannot
      produce a token for the tenant this credential is pinned to.

    Note what this does **not** tell you. A successful return proves only
    that the engineer is signed in to the right tenant — it says nothing
    about whether they may actually call Azure OpenAI. Azure AD hands a
    Cognitive Services token to any authenticated member of the tenant; the
    role assignment is enforced by the resource at call time. Use
    `verify_access()` when the question is "can this person work", and this
    only when the question is strictly "can this person authenticate".

    This function never catches either error into a generic failure — that
    is left entirely to the caller, which needs the type to report the right
    message to the engineer.
    """
    provider = _get_token_provider()
    return await provider()


async def _get(url: str, headers: dict[str, str]) -> httpx.Response:
    """The one HTTP call `verify_access` makes, behind its own name.

    Kept as a module-level function purely so tests can replace it, the same
    way they already replace `AzureCliCredential` and
    `get_bearer_token_provider` above. That keeps every test in this package
    free of the network while still exercising the part that carries the
    real logic -- reading a verdict out of a status code.
    """
    async with httpx.AsyncClient(timeout=_PROBE_TIMEOUT_SECONDS) as client:
        return await client.get(url, headers=headers)


async def verify_access() -> None:
    """Prove the signed-in engineer can actually *call* Azure OpenAI, not
    merely authenticate to the tenant (tickets #94, #95).

    Returns None when access is confirmed. Raises, by type:

    - `CredentialUnavailableError` — the Azure CLI isn't installed, or
      nobody is signed in. (From `attempt_token`.)
    - `ClientAuthenticationError` — signed in, but to the wrong tenant.
      (From `attempt_token`.)
    - `AzureAccessDeniedError` — signed in to the right tenant, but lacking
      the Azure OpenAI role assignment. **Only a real call can discover
      this**, which is the whole reason this function exists on top of
      `attempt_token`.
    - `AzureAccessUnverifiedError` — no verdict was reachable.

    Only `401` and `403` are read as a refusal. Every other unexpected
    answer — a connection failure, a `404`, a `500` — is
    `AzureAccessUnverifiedError` instead, and callers let the engineer
    proceed on it. That asymmetry is deliberate: this check is new, and a
    new check that stops a runner which would have worked is worse than the
    gap it closes. It may only ever block on an unambiguous refusal.

    The request is a plain listing, so it consumes no tokens and costs
    nothing. It is authorised by the same role a completion needs, which is
    what makes it a valid stand-in for one.
    """
    token = await attempt_token()

    endpoint = settings.AZURE_OPENAI_ENDPOINT.rstrip("/")
    if not endpoint:
        raise AzureAccessUnverifiedError(
            "AZURE_OPENAI_ENDPOINT is not set, so there is nothing to call"
        )

    url = f"{endpoint}/openai/models?api-version={settings.AZURE_OPENAI_API_VERSION}"
    try:
        response = await _get(url, {"Authorization": f"Bearer {token}"})
    except httpx.HTTPError as exc:
        raise AzureAccessUnverifiedError(f"could not reach {endpoint}: {exc}") from exc

    if response.status_code in (401, 403):
        raise AzureAccessDeniedError(
            f"{endpoint} refused the call with HTTP {response.status_code}"
        )
    if response.is_success:
        return
    raise AzureAccessUnverifiedError(
        f"{endpoint} answered HTTP {response.status_code}, which is neither a "
        "success nor a refusal"
    )
