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

from azure.core.exceptions import ClientAuthenticationError  # noqa: F401 (re-exported for callers)
from azure.identity import CredentialUnavailableError  # noqa: F401 (re-exported for callers)
from azure.identity.aio import AzureCliCredential, get_bearer_token_provider

from wso2_runner.config import settings

# Resource scope for Azure OpenAI / Azure Cognitive Services data-plane calls.
_COGNITIVE_SERVICES_SCOPE = "https://cognitiveservices.azure.com/.default"

TokenProvider = Callable[[], Awaitable[str]]

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
    if settings.AZURE_OPENAI_AUTH_MODE == "api_key":
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
      rejected: e.g. signed in to the wrong tenant, or signed in but
      lacking the Azure OpenAI role assignment.

    This function never catches either into a generic failure — that is
    left entirely to the caller, which needs the type to report the right
    message to the engineer.
    """
    provider = _get_token_provider()
    return await provider()
