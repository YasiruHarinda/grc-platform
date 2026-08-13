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
    azure_credential._credential = None
    azure_credential._token_provider = None
    yield
    settings.AZURE_OPENAI_AUTH_MODE = original_mode
    settings.AZURE_TENANT_ID = original_tenant
    settings.AZURE_OPENAI_API_KEY = original_key
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
