"""Unit tests for `wso2_runner.azure_credential`.

The vendor Azure SDK is faked at the module boundary — `AzureCliCredential`
and `get_bearer_token_provider`, the two names `wso2_runner.azure_credential`
imports from `azure.identity(.aio)` — so no test here needs the Azure CLI,
network access, or a real tenant. Module functions are still called and
asserted on directly; only the vendor edge is replaced.

`settings` is the same process-wide singleton `tests/test_cli.py` isolates
mutations to; the autouse fixture below follows that pattern. It also resets
`azure_credential`'s own module-scope credential/token-provider cache
between tests, since production code deliberately builds that once and
reuses it for the life of the process — exactly the behaviour under test in
the "repeated calls" case below, so each test needs to start from a clean
cache rather than reusing whatever a previous test built.

The runner has no `asyncio` test plugin installed (see `tests/test_loop.py`
for prior art), so async module functions are driven with `asyncio.run`
rather than `async def test_...`.
"""
import asyncio

import httpx
import pytest

import wso2_runner.azure_credential as azure_credential
from wso2_runner.azure_credential import ClientAuthenticationError, CredentialUnavailableError
from wso2_runner.config import settings


class FakeCredential:
    """Stand-in for azure.identity.aio.AzureCliCredential.

    `outcome` is test-controlled: leave it a string and the fake token
    provider "succeeds" with that string as the token; set it to an
    exception instance and the provider raises it instead, letting tests
    drive the two vendor failure modes without a real credential.
    """

    def __init__(self, *, tenant_id: str = ""):
        self.tenant_id = tenant_id
        self.outcome: object = "fake-token"


class FakeVendor:
    """What the `fake_vendor_credential` fixture hands to each test: the
    `FakeCredential` instances constructed during the test, in construction
    order, plus a count of how many times a token provider was built —
    together enough to assert both "the tenant reached the credential" and
    "the credential/provider is reused, not rebuilt"."""

    def __init__(self):
        self.created: list[FakeCredential] = []
        self.provider_builds = 0


@pytest.fixture
def fake_vendor_credential(monkeypatch):
    vendor = FakeVendor()

    def fake_credential_ctor(*, tenant_id: str = ""):
        cred = FakeCredential(tenant_id=tenant_id)
        vendor.created.append(cred)
        return cred

    def fake_get_bearer_token_provider(credential, scope):
        vendor.provider_builds += 1

        async def _provider():
            if isinstance(credential.outcome, BaseException):
                raise credential.outcome
            return credential.outcome

        return _provider

    monkeypatch.setattr(azure_credential, "AzureCliCredential", fake_credential_ctor)
    monkeypatch.setattr(azure_credential, "get_bearer_token_provider", fake_get_bearer_token_provider)
    return vendor


@pytest.fixture(autouse=True)
def _isolate_settings_and_module_cache():
    """Restore the settings singleton and reset this module's module-scope
    credential cache after every test, so no test leaks state — mutated
    settings or an already-built credential — into the next one."""
    original_mode = settings.AZURE_OPENAI_AUTH_MODE
    original_tenant = settings.AZURE_TENANT_ID
    original_key = settings.AZURE_OPENAI_API_KEY
    original_endpoint = settings.AZURE_OPENAI_ENDPOINT
    azure_credential._credential = None
    azure_credential._token_provider = None
    yield
    settings.AZURE_OPENAI_AUTH_MODE = original_mode
    settings.AZURE_TENANT_ID = original_tenant
    settings.AZURE_OPENAI_API_KEY = original_key
    settings.AZURE_OPENAI_ENDPOINT = original_endpoint
    azure_credential._credential = None
    azure_credential._token_provider = None


# ── (a) entra mode produces a token provider ────────────────────────────


def test_entra_mode_produces_a_token_provider(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-a"

    auth = azure_credential.resolve_llm_auth()

    assert callable(auth)


# ── (b) the configured tenant reaches the credential ────────────────────


def test_configured_tenant_reaches_the_credential(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-b"

    azure_credential.resolve_llm_auth()

    assert len(fake_vendor_credential.created) == 1
    assert fake_vendor_credential.created[0].tenant_id == "tenant-b"


# ── (c) api_key mode is opt-in only, never an implicit fallback ─────────


def test_api_key_mode_selected_when_explicitly_configured(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "api_key"
    settings.AZURE_OPENAI_API_KEY = "sk-something"

    auth = azure_credential.resolve_llm_auth()

    assert auth is None
    # api_key mode must never touch the entra credential at all.
    assert fake_vendor_credential.created == []


def test_present_api_key_does_not_silently_activate_api_key_mode(fake_vendor_credential):
    """A stale AZURE_OPENAI_API_KEY sitting in someone's .env must not
    quietly keep working — mode "entra" must win even though a key is
    present, because the mode is never inferred from what's present."""
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-c"
    settings.AZURE_OPENAI_API_KEY = "sk-stale-leftover-key"

    auth = azure_credential.resolve_llm_auth()

    assert auth is not None
    assert callable(auth)


# ── (d) entra mode with an empty tenant fails clearly ────────────────────


def test_entra_mode_with_empty_tenant_fails_clearly(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = ""

    with pytest.raises(RuntimeError, match="AZURE_TENANT_ID"):
        azure_credential.resolve_llm_auth()

    # Must fail before ever constructing a credential with an empty tenant.
    assert fake_vendor_credential.created == []


# ── (e) CredentialUnavailableError / ClientAuthenticationError distinct ──


def test_credential_unavailable_error_surfaces_by_type(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-d"
    azure_credential.resolve_llm_auth()
    fake_vendor_credential.created[0].outcome = CredentialUnavailableError("az cli not found")

    with pytest.raises(CredentialUnavailableError):
        asyncio.run(azure_credential.attempt_token())


def test_client_authentication_error_surfaces_by_type(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-e"
    azure_credential.resolve_llm_auth()
    fake_vendor_credential.created[0].outcome = ClientAuthenticationError("token request rejected")

    with pytest.raises(ClientAuthenticationError):
        asyncio.run(azure_credential.attempt_token())


def test_the_two_error_types_are_not_confused_with_each_other():
    """`CredentialUnavailableError` is actually a *subclass* of
    `ClientAuthenticationError` in the vendor library's own hierarchy, so a
    handler that only checks the broader type would wrongly report every
    "CLI not installed / not logged in" case as a rejected-auth case too.
    Prove the narrower type is distinguishable on its own, independent of
    this module — this is the exact confusion the ticket rules out."""
    assert issubclass(CredentialUnavailableError, ClientAuthenticationError)

    def classify(exc: BaseException) -> str:
        # Mirrors how a caller (e.g. `doctor`, #95) must order its checks:
        # the narrower type first, or it is silently swallowed by the wider one.
        if isinstance(exc, CredentialUnavailableError):
            return "unavailable"
        if isinstance(exc, ClientAuthenticationError):
            return "rejected"
        return "generic"

    assert classify(CredentialUnavailableError("no cli")) == "unavailable"
    assert classify(ClientAuthenticationError("rejected")) == "rejected"


# ── (f) repeated calls reuse one credential rather than rebuilding it ───


def test_repeated_calls_reuse_one_credential(fake_vendor_credential):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-f"

    first = azure_credential.resolve_llm_auth()
    second = azure_credential.resolve_llm_auth()
    asyncio.run(azure_credential.attempt_token())

    assert first is second
    assert len(fake_vendor_credential.created) == 1
    assert fake_vendor_credential.provider_builds == 1


# ── (h) verify_access: proving access, not merely authentication ────────
#
# These are the tests for the gap a token check cannot see. Azure AD issues
# a Cognitive Services token to any authenticated member of the tenant, so
# an engineer without the role assignment authenticates perfectly and is
# refused by the *resource* instead. Only a real call reaches that, which is
# why verify_access exists on top of attempt_token.
#
# `_get` is replaced rather than the network being reached -- the same
# fake-at-the-vendor-boundary approach the rest of this file uses. What is
# actually under test is the classification: which status code means what.


@pytest.fixture
def probe(monkeypatch):
    """Replaces azure_credential._get, records the request it was given, and
    replies with whatever the test asks for -- an httpx.Response to return,
    or an exception to raise."""
    calls = []
    box = {"outcome": httpx.Response(200, json={"data": []})}

    async def fake_get(url, headers):
        calls.append((url, headers))
        outcome = box["outcome"]
        if isinstance(outcome, BaseException):
            raise outcome
        return outcome

    monkeypatch.setattr(azure_credential, "_get", fake_get)
    return calls, box


def _entra(endpoint="https://example.openai.azure.com"):
    settings.AZURE_OPENAI_AUTH_MODE = "entra"
    settings.AZURE_TENANT_ID = "tenant-h"
    settings.AZURE_OPENAI_ENDPOINT = endpoint


def test_verify_access_passes_when_the_resource_accepts_the_call(fake_vendor_credential, probe):
    _entra()

    assert asyncio.run(azure_credential.verify_access()) is None


def test_verify_access_sends_the_token_as_a_bearer_to_the_configured_endpoint(
    fake_vendor_credential, probe
):
    calls, _ = probe
    _entra()

    asyncio.run(azure_credential.verify_access())

    url, headers = calls[0]
    assert url.startswith("https://example.openai.azure.com/openai/models")
    assert headers["Authorization"] == "Bearer fake-token"


def test_verify_access_reports_a_forbidden_call_as_a_missing_role(fake_vendor_credential, probe):
    """403 is the whole point of this function: authenticated, and refused."""
    _, box = probe
    _entra()
    box["outcome"] = httpx.Response(403, json={"error": "PermissionDenied"})

    with pytest.raises(azure_credential.AzureAccessDeniedError):
        asyncio.run(azure_credential.verify_access())


def test_verify_access_reports_an_unauthorized_call_as_a_missing_role(fake_vendor_credential, probe):
    _, box = probe
    _entra()
    box["outcome"] = httpx.Response(401)

    with pytest.raises(azure_credential.AzureAccessDeniedError):
        asyncio.run(azure_credential.verify_access())


def test_verify_access_cannot_judge_without_an_endpoint(fake_vendor_credential, probe):
    calls, _ = probe
    _entra(endpoint="")

    with pytest.raises(azure_credential.AzureAccessUnverifiedError):
        asyncio.run(azure_credential.verify_access())
    assert calls == []


def test_verify_access_treats_an_unreachable_endpoint_as_unverified_not_denied(
    fake_vendor_credential, probe
):
    """A laptop on bad wifi must never be told to go and ask an
    administrator for a role it already holds."""
    _, box = probe
    _entra()
    box["outcome"] = httpx.ConnectError("no route to host")

    with pytest.raises(azure_credential.AzureAccessUnverifiedError):
        asyncio.run(azure_credential.verify_access())


def test_verify_access_treats_an_unexpected_status_as_unverified_not_denied(
    fake_vendor_credential, probe
):
    """Only 401 and 403 are unambiguous refusals. Anything else -- a 500, a
    404 from an api-version that has no such route -- leaves the question
    open, and this check may only ever block on an unambiguous refusal."""
    _, box = probe
    _entra()
    box["outcome"] = httpx.Response(500, text="boom")

    with pytest.raises(azure_credential.AzureAccessUnverifiedError):
        asyncio.run(azure_credential.verify_access())


def test_verify_access_never_probes_when_authentication_itself_failed(
    fake_vendor_credential, probe
):
    """The token failure surfaces by its own type, unchanged, and no call is
    attempted -- there is nothing to authorise without a token."""
    calls, _ = probe
    _entra()
    fake_vendor_credential.created  # credential is built lazily below
    azure_credential.resolve_llm_auth()
    fake_vendor_credential.created[0].outcome = CredentialUnavailableError("no az cli")

    with pytest.raises(CredentialUnavailableError):
        asyncio.run(azure_credential.verify_access())
    assert calls == []


def test_verify_access_surfaces_a_wrong_tenant_rejection_by_type(fake_vendor_credential, probe):
    calls, _ = probe
    _entra()
    azure_credential.resolve_llm_auth()
    fake_vendor_credential.created[0].outcome = ClientAuthenticationError("wrong tenant")

    with pytest.raises(ClientAuthenticationError):
        asyncio.run(azure_credential.verify_access())
    assert calls == []
