"""Unit tests for `wso2_runner.cli` — the Typer CLI (`configure`, `doctor`, `start`).

`start` and `doctor` both lazily import `wso2_runner.loop` / touch
`wso2_runner.config.settings` *inside* the command body, so the CLI module
itself has no heavy imports at collection time. `wso2_runner.loop`, however,
transitively imports `wso2_runner.agent`, which imports the real
`browser_use` package — not installed in this environment. To make
`wso2_runner.loop` importable (so its `run_forever` can be patched at the
right site) we inject a fake `wso2_runner.agent` module into `sys.modules`
before importing `wso2_runner.loop`, mirroring the trick the loop tests use.

None of these tests ever run the real polling loop, hit the network, or
open a browser: `run_forever` is replaced with a recording async stub,
`httpx.get` is stubbed for the `doctor` checks, and `configure`'s
`CONFIG_DIR`/`CONFIG_FILE` are redirected into `tmp_path`.

`start`'s Azure start-up gate (ticket #94) calls
`wso2_runner.azure_credential.verify_access()` before it ever reaches
`run_forever` — a real access check that signs in *and* calls the endpoint,
not just a token fetch. `azure_credential` imports no `browser_use`, so it
needs none of the sys.modules trickery above — `fake_run_forever` below stubs
`verify_access()` to succeed by default so every pre-existing `start` test
that merely sets `AGENT_PROVIDER = "azure"` still reaches `run_forever`
exactly as it did before this gate existed. The dedicated Azure gate tests
further down override that stub to exercise the failure paths.
"""
import os
import subprocess
import sys
import types
from pathlib import Path

import httpx
import pytest
from typer.testing import CliRunner

import wso2_runner.cli as cli
import wso2_runner.config as config_mod
from wso2_runner import oauth
from wso2_runner.azure_credential import ClientAuthenticationError, CredentialUnavailableError
from wso2_runner.config import settings

runner = CliRunner()

# Root of the runner package — cwd for the subprocess test below, so a plain
# `import wso2_runner...` resolves the same way it does for every other test.
_RUNNER_ROOT = Path(__file__).resolve().parent.parent


@pytest.fixture
def fake_run_forever(monkeypatch):
    """Import wso2_runner.loop (faking out browser_use via a stub agent
    module) and replace its run_forever with a recorder that returns
    immediately, patched at the site `start` actually imports it from.
    Also stubs the Azure start-up gate's access check to succeed, so tests
    that don't care about Azure auth still reach run_forever.

    Returns a list that the recorder appends
    (cloud_url, user_email, poll_interval) tuples to.
    """
    if "wso2_runner.agent" not in sys.modules:
        fake_agent = types.ModuleType("wso2_runner.agent")
        fake_agent.execute_task = lambda *a, **k: None
        fake_agent.open_login_browser = lambda *a, **k: None
        fake_agent.reset_browser = lambda *a, **k: None
        monkeypatch.setitem(sys.modules, "wso2_runner.agent", fake_agent)

    import wso2_runner.loop as loop

    calls = []

    async def _fake_run_forever(cloud_url=None, user_email=None, poll_interval=None):
        calls.append((cloud_url, user_email, poll_interval))

    monkeypatch.setattr(loop, "run_forever", _fake_run_forever)

    import wso2_runner.azure_credential as azure_credential

    async def _fake_verify_access():
        return None

    monkeypatch.setattr(azure_credential, "verify_access", _fake_verify_access)

    return calls


@pytest.fixture(autouse=True)
def _restore_agent_provider():
    """`settings` is a process-wide singleton `cli.py` reaches into by
    importing it fresh each call; tests that mutate AGENT_PROVIDER (or the
    Azure auth mode) must not leak that mutation into other tests in this
    file or other test files."""
    original_provider = settings.AGENT_PROVIDER
    original_auth_mode = settings.AZURE_OPENAI_AUTH_MODE
    yield
    settings.AGENT_PROVIDER = original_provider
    settings.AZURE_OPENAI_AUTH_MODE = original_auth_mode


# ── start: user-resolution precedence ───────────────────────────────────


def test_start_resolves_email_from_positional_argument(fake_run_forever):
    settings.AGENT_PROVIDER = "azure"

    result = runner.invoke(cli.app, ["start", "positional@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "positional@wso2.com", None)]


def test_start_resolves_email_from_user_flag(fake_run_forever):
    settings.AGENT_PROVIDER = "azure"

    result = runner.invoke(cli.app, ["start", "--user", "flag@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "flag@wso2.com", None)]


def test_start_positional_argument_wins_over_user_flag(fake_run_forever):
    """cli.py does `user = email or user` — the positional argument takes
    precedence over --user when both are given."""
    settings.AGENT_PROVIDER = "azure"

    result = runner.invoke(cli.app, ["start", "positional@wso2.com", "--user", "flag@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "positional@wso2.com", None)]


def test_start_resolves_email_from_user_email_env_var(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "azure"
    monkeypatch.setenv("USER_EMAIL", "envvar@wso2.com")

    result = runner.invoke(cli.app, ["start"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "envvar@wso2.com", None)]


def test_start_email_is_none_when_nothing_given(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "azure"
    monkeypatch.delenv("USER_EMAIL", raising=False)

    result = runner.invoke(cli.app, ["start"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, None, None)]


def test_start_passes_server_and_interval_through(fake_run_forever):
    settings.AGENT_PROVIDER = "azure"

    result = runner.invoke(
        cli.app,
        ["start", "someone@wso2.com", "--server", "https://cloud.example.com", "--interval", "7.5"],
    )

    assert result.exit_code == 0
    assert fake_run_forever == [("https://cloud.example.com", "someone@wso2.com", 7.5)]


# ── start: AGENT_PROVIDER gate ──────────────────────────────────────────


def test_start_exits_nonzero_and_prompts_configure_when_no_provider(fake_run_forever):
    settings.AGENT_PROVIDER = ""

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "wso2-runner configure" in result.output
    # run_forever must never be reached when the provider gate blocks.
    assert fake_run_forever == []


def test_start_proceeds_to_run_forever_when_provider_configured(fake_run_forever):
    settings.AGENT_PROVIDER = "anthropic"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


# ── start: ASGARDEO_ORG gate ─────────────────────────────────────────────
#
# ASGARDEO_ORG used to be a required pydantic field with no default, so a
# missing value crashed at import time with a raw traceback before `start`
# ever ran. It now defaults to "", and `start` is responsible for catching
# a missing value itself and naming it in plain language, the same way it
# already does for AGENT_PROVIDER above.


def test_start_exits_nonzero_and_names_asgardeo_org_when_missing(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "anthropic"
    monkeypatch.setattr(settings, "ASGARDEO_ORG", "")

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "ASGARDEO_ORG" in result.output
    # run_forever must never be reached when the org gate blocks.
    assert fake_run_forever == []


def test_start_proceeds_to_run_forever_when_asgardeo_org_configured(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "anthropic"
    monkeypatch.setattr(settings, "ASGARDEO_ORG", "acme")

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


# ── start: Azure auth gate (entra mode, ticket #94) ─────────────────────


def test_start_azure_entra_mode_checks_azure_auth_before_run_forever(fake_run_forever):
    """Default mode is "entra" — the gate runs, succeeds (via the
    fake_run_forever fixture's default stub), and start proceeds exactly as
    it did before this gate existed."""
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "entra"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_azure_api_key_mode_skips_the_azure_gate(fake_run_forever, monkeypatch):
    """api_key mode must behave exactly as it did before this ticket — no
    Azure token check at all, even if acquiring one would fail."""
    import wso2_runner.azure_credential as azure_credential

    async def _boom():
        raise CredentialUnavailableError("must never be called in api_key mode")

    monkeypatch.setattr(azure_credential, "verify_access", _boom)
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "api_key"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_exits_nonzero_when_azure_cli_unavailable(fake_run_forever, monkeypatch):
    """No Azure CLI installed / nobody signed in — the narrower
    CredentialUnavailableError case. Must exit non-zero, name `az login`,
    and never reach run_forever (no browser opens, no task is consumed)."""
    import wso2_runner.azure_credential as azure_credential

    async def _unavailable():
        raise CredentialUnavailableError("az cli not found")

    monkeypatch.setattr(azure_credential, "verify_access", _unavailable)
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "entra"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "az login" in result.output
    assert fake_run_forever == []


def test_start_exits_nonzero_when_azure_session_rejected(fake_run_forever, monkeypatch):
    """Signed in, but authentication is rejected — e.g. wrong tenant, or
    missing the Azure OpenAI role. Must exit non-zero, name `az login`, and
    never reach run_forever."""
    import wso2_runner.azure_credential as azure_credential

    async def _rejected():
        raise ClientAuthenticationError("token request rejected")

    monkeypatch.setattr(azure_credential, "verify_access", _rejected)
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "entra"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "az login" in result.output
    assert fake_run_forever == []


def test_start_exits_nonzero_when_azure_role_is_missing(fake_run_forever, monkeypatch):
    """Signed in correctly and still refused by the resource. This is the
    case the old token-only gate let straight through: the runner started,
    took a task, opened a browser, and failed on the first LLM call. It must
    now stop here, and must not suggest `az login`."""
    import wso2_runner.azure_credential as azure_credential
    from wso2_runner.azure_credential import AzureAccessDeniedError

    async def _denied():
        raise AzureAccessDeniedError("endpoint refused the call with HTTP 403")

    monkeypatch.setattr(azure_credential, "verify_access", _denied)
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "entra"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "not allowed to call Azure OpenAI" in result.output
    assert "az login" not in result.output
    assert fake_run_forever == []


def test_start_proceeds_when_azure_access_cannot_be_verified(fake_run_forever, monkeypatch):
    """An inconclusive probe -- no endpoint set, or the network is down --
    must warn and let the runner start. A new check that stops a runner
    which would have worked is worse than the gap it closes."""
    import wso2_runner.azure_credential as azure_credential
    from wso2_runner.azure_credential import AzureAccessUnverifiedError

    async def _unverified():
        raise AzureAccessUnverifiedError("could not reach the endpoint")

    monkeypatch.setattr(azure_credential, "verify_access", _unverified)
    settings.AGENT_PROVIDER = "azure"
    settings.AZURE_OPENAI_AUTH_MODE = "entra"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert "Could not confirm Azure OpenAI access" in result.output
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_non_azure_provider_never_reaches_the_azure_gate(fake_run_forever, monkeypatch):
    """A provider other than "azure" must never attempt an Azure token
    check, regardless of AZURE_OPENAI_AUTH_MODE."""
    import wso2_runner.azure_credential as azure_credential

    async def _boom():
        raise CredentialUnavailableError("must never be called for a non-azure provider")

    monkeypatch.setattr(azure_credential, "verify_access", _boom)
    settings.AGENT_PROVIDER = "anthropic"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_keyboard_interrupt_exits_zero_and_prints_stopped(monkeypatch):
    """A Ctrl-C during the loop is a clean shutdown, not a crash: `start`
    catches KeyboardInterrupt around asyncio.run and exits 0."""
    if "wso2_runner.agent" not in sys.modules:
        fake_agent = types.ModuleType("wso2_runner.agent")
        fake_agent.execute_task = lambda *a, **k: None
        fake_agent.open_login_browser = lambda *a, **k: None
        fake_agent.reset_browser = lambda *a, **k: None
        monkeypatch.setitem(sys.modules, "wso2_runner.agent", fake_agent)

    import wso2_runner.loop as loop

    async def _interrupting_run_forever(cloud_url=None, user_email=None, poll_interval=None):
        raise KeyboardInterrupt

    monkeypatch.setattr(loop, "run_forever", _interrupting_run_forever)
    settings.AGENT_PROVIDER = "anthropic"

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert "[runner] Stopped." in result.output


# ── configure ────────────────────────────────────────────────────────────


def test_configure_writes_config_file_from_prompts(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # provider=anthropic, model=<accept default>, api key, monitor=<accept default>
    input_text = "anthropic\n\nsk-test-key\n1\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    assert cfg_file.exists()
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=anthropic" in content
    assert "ANTHROPIC_API_KEY=sk-test-key" in content
    assert "SCREENSHOT_MONITOR=1" in content


def test_configure_azure_provider_writes_endpoint_deployment_and_tenant(monkeypatch, tmp_path):
    """Azure setup no longer hands out a shared API key — see ticket #95.
    The wizard writes endpoint, deployment and tenant, and points the
    engineer at `az login` instead of prompting for a secret."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # provider=azure (default, just press enter), model=<default>,
    # endpoint, deployment=<default>, tenant ID, monitor=<default>
    input_text = "\n\nhttps://myorg.openai.azure.com\n\nsome-tenant-id\n1\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=azure" in content
    assert "AZURE_OPENAI_ENDPOINT=https://myorg.openai.azure.com" in content
    assert "AZURE_TENANT_ID=some-tenant-id" in content
    assert "AZURE_OPENAI_API_KEY" not in content
    assert "az login" in result.output


def test_configure_ollama_provider_needs_no_api_key(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # provider=ollama, model=<default>, monitor=<default>
    input_text = "ollama\n\n1\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=ollama" in content
    assert "no API key" in result.output


def test_configure_gemini_provider_writes_api_key(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # provider=gemini, model=<default>, api key, monitor=2 (triggers the
    # "drag the agent's Chrome window" tip branch)
    input_text = "gemini\n\nsk-gemini-key\n2\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=gemini" in content
    assert "GEMINI_API_KEY=sk-gemini-key" in content
    assert "SCREENSHOT_MONITOR=2" in content
    assert "drag the agent's Chrome window to monitor 2" in result.output


def test_configure_runs_on_a_brand_new_machine(tmp_path):
    """The bug this whole ticket exists for: on a machine with no
    ~/.wso2-runner/.env, `configure` crashed with a raw pydantic traceback
    before its first prompt, because it does `from wso2_runner.config
    import CONFIG_DIR, CONFIG_FILE` (see `configure()` in cli.py), which
    re-runs config.py's module-level `settings = RunnerSettings()`.

    The tests above can't reproduce that: `wso2_runner.config` is already
    imported by the time they run, and conftest.py has already seeded
    ASGARDEO_ORG into the environment (see its docstring). Only a real
    subprocess, with HOME pointed at an empty directory, proves the wizard
    can actually start on a genuinely fresh machine.
    """
    env = {
        **os.environ,
        "HOME": str(tmp_path),
        # See test_import_never_raises_on_a_machine_with_no_config in
        # tests/test_config.py for why PYTHONPATH has to be forwarded
        # explicitly once HOME points somewhere else.
        "PYTHONPATH": os.pathsep.join(p for p in sys.path if p),
    }
    env.pop("ASGARDEO_ORG", None)

    # provider=ollama, model=<default>, monitor=<default> — needs no API key.
    result = subprocess.run(
        [sys.executable, "-c", "import sys; sys.argv = ['wso2-runner', 'configure']; from wso2_runner.cli import app; app()"],
        cwd=_RUNNER_ROOT,
        env=env,
        input="ollama\n\n1\n",
        capture_output=True,
        text=True,
    )

    assert result.returncode == 0, result.stderr
    config_file = tmp_path / ".wso2-runner" / ".env"
    assert config_file.exists()
    assert "AGENT_PROVIDER=ollama" in config_file.read_text()


# ── doctor ───────────────────────────────────────────────────────────────


class _FakeResponse:
    def __init__(self, data):
        self._data = data

    def json(self):
        return self._data


def test_doctor_reports_backend_health_and_missing_client_id(monkeypatch):
    """Drives `doctor` end-to-end with httpx.get stubbed so nothing hits the
    network, browser-use/playwright genuinely missing (caught by the
    command's own try/except), and no cached Asgardeo session."""

    def fake_get(url, *a, **k):
        assert url == "http://cloud.test/health"
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "sk-x")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Backend connectivity: http://cloud.test" in result.output
    assert "{'status': 'ok'}" in result.output
    assert "ASGARDEO_CLIENT_ID is not set" in result.output


def test_doctor_reports_missing_asgardeo_org(monkeypatch):
    """Mirrors test_doctor_reports_backend_health_and_missing_client_id
    above, for ASGARDEO_ORG: it must be reported the same way, as a plain
    line naming the setting, never a traceback.

    ASGARDEO_CLIENT_ID is set (unlike that other test) so this exercises
    the ASGARDEO_ORG check specifically, rather than short-circuiting on a
    missing client ID first. `oauth.has_cached_session` is stubbed to keep
    this test from depending on whether the machine running it happens to
    have a real cached Asgardeo session on disk.
    """

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_ORG", "")
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "some-client-id")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "sk-x")
    monkeypatch.setattr(oauth, "has_cached_session", lambda: False)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "ASGARDEO_ORG is not set" in result.output


def test_doctor_reports_missing_anthropic_key(monkeypatch):
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "ANTHROPIC_API_KEY is not set" in result.output


def test_doctor_reports_missing_gemini_key(monkeypatch):
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "gemini")
    monkeypatch.setattr(settings, "GEMINI_API_KEY", "")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "GEMINI_API_KEY is not set" in result.output


def test_doctor_reports_missing_azure_key_in_api_key_mode(monkeypatch):
    """api_key mode keeps today's behaviour — ticket #95 only replaces the
    check in entra mode, the default."""

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "api_key")
    monkeypatch.setattr(settings, "AZURE_OPENAI_API_KEY", "")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "AZURE_OPENAI_API_KEY is not set" in result.output


def test_doctor_reports_azure_key_present_in_api_key_mode(monkeypatch):
    """api_key mode with a non-empty key keeps the old, shallow "✓ Key
    present" report — it never attempts a real token in this mode."""

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "api_key")
    monkeypatch.setattr(settings, "AZURE_OPENAI_API_KEY", "sk-azure-key")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "✓ Key present" in result.output


# ── doctor: Azure entra-mode LLM check, ticket #95 ──────────────────────
#
# These fake verify_access() at the wso2_runner.azure_credential module
# boundary — the same seam the `start` gate tests above use — so no test
# here needs the Azure CLI, network access, or a real tenant.


def test_doctor_azure_entra_cli_not_installed(monkeypatch):
    """CredentialUnavailableError plus no `az` on PATH: the one case where
    the extra shutil.which signal lets us name the problem precisely."""
    import wso2_runner.azure_credential as azure_credential

    async def _unavailable():
        raise CredentialUnavailableError("az cli not found")

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _unavailable)
    monkeypatch.setattr("shutil.which", lambda cmd: None)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Azure CLI is not installed" in result.output
    assert "az login" in result.output


def test_doctor_azure_entra_nobody_signed_in(monkeypatch):
    """CredentialUnavailableError plus `az` present on PATH: installed, but
    no cached CLI session — a different fix from "not installed"."""
    import wso2_runner.azure_credential as azure_credential

    async def _unavailable():
        raise CredentialUnavailableError("no cached token")

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _unavailable)
    monkeypatch.setattr("shutil.which", lambda cmd: "/usr/bin/az")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "nobody is signed in" in result.output
    assert "az login" in result.output


def test_doctor_azure_entra_authentication_rejected(monkeypatch):
    """ClientAuthenticationError now means one thing only: the wrong
    tenant. The credential is pinned to AZURE_TENANT_ID, so a rejection at
    token time is the CLI holding a session for some other tenant. Missing
    the role assignment is a separate state -- it cannot raise here,
    because Azure AD issues the token regardless and the resource is what
    refuses. See test_doctor_azure_entra_missing_role_assignment."""
    import wso2_runner.azure_credential as azure_credential

    async def _rejected():
        raise ClientAuthenticationError("token request rejected")

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _rejected)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "wrong tenant" in result.output
    assert "az account show" in result.output


def test_doctor_azure_entra_working(monkeypatch):
    """The success state: a real call to Azure OpenAI was authorised — no
    more '✓ Key present' string presence in entra mode, and no longer
    satisfied by merely acquiring a token."""
    import wso2_runner.azure_credential as azure_credential

    async def _ok():
        return "fake-token"

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _ok)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "✓ Azure OpenAI access works" in result.output
    assert "Key present" not in result.output


def test_doctor_azure_entra_missing_role_assignment(monkeypatch):
    """The state a token check alone can never reach. Azure AD hands a
    Cognitive Services token to any member of the tenant, so an engineer
    without the role authenticates perfectly and is refused by the resource
    instead. doctor must report that as its own problem, and must not tell
    them to run `az login` -- nothing they can do at a terminal fixes it."""
    import wso2_runner.azure_credential as azure_credential
    from wso2_runner.azure_credential import AzureAccessDeniedError

    async def _denied():
        raise AzureAccessDeniedError("endpoint refused the call with HTTP 403")

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _denied)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "not allowed to call Azure OpenAI" in result.output
    assert "administrator" in result.output
    assert "az login` will not fix this" in result.output


def test_doctor_azure_entra_access_unverified(monkeypatch):
    """No endpoint configured, or it could not be reached. Reported as an
    open question rather than a refusal, so nobody is sent to an
    administrator over a network problem."""
    import wso2_runner.azure_credential as azure_credential
    from wso2_runner.azure_credential import AzureAccessUnverifiedError

    async def _unverified():
        raise AzureAccessUnverifiedError("AZURE_OPENAI_ENDPOINT is not set")

    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_AUTH_MODE", "entra")
    monkeypatch.setattr(azure_credential, "verify_access", _unverified)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "could not be confirmed" in result.output
    assert "not allowed to call" not in result.output


def test_doctor_ollama_not_running_is_reported_not_raised(monkeypatch):
    def fake_get(url, *a, **k):
        if url.endswith("/health"):
            return _FakeResponse({"status": "ok"})
        raise httpx.ConnectError("refused")

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "ollama")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Ollama not running on localhost:11434" in result.output


def test_doctor_reports_authenticated_user_when_session_cached(monkeypatch):
    """When a cached Asgardeo session exists, doctor exchanges it for a
    token and calls /api/me — this pins that success path (not just the
    "not signed in yet" branch)."""

    def fake_get(url, *a, **k):
        if url.endswith("/health"):
            return _FakeResponse({"status": "ok"})
        assert url.endswith("/api/me")
        assert k["headers"]["Authorization"] == "Bearer test-token"
        return _FakeResponse({"email": "someone@wso2.com"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "client-123")
    monkeypatch.setattr(settings, "ASGARDEO_ORG", "test-org")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "")
    monkeypatch.setattr(oauth, "has_cached_session", lambda: True)
    monkeypatch.setattr(oauth, "get_access_token", lambda org, cid: "test-token")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "{'email': 'someone@wso2.com'}" in result.output


def test_doctor_reports_not_signed_in_when_no_cached_session(monkeypatch):
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "client-123")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "")
    monkeypatch.setattr(oauth, "has_cached_session", lambda: False)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Not signed in yet. Run `wso2-runner start` first" in result.output


def test_doctor_ollama_running_but_model_missing(monkeypatch):
    def fake_get(url, *a, **k):
        if url.endswith("/health"):
            return _FakeResponse({"status": "ok"})
        return _FakeResponse({"models": [{"name": "llama3"}]})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "ollama")
    monkeypatch.setattr(settings, "AGENT_MODEL", "qwen2.5:7b")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Ollama running but model qwen2.5:7b not found" in result.output


def test_doctor_ollama_running_with_model_found(monkeypatch):
    def fake_get(url, *a, **k):
        if url.endswith("/health"):
            return _FakeResponse({"status": "ok"})
        assert url == "http://localhost:11434/api/tags"
        return _FakeResponse({"models": [{"name": "qwen2.5:7b"}]})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "ollama")
    monkeypatch.setattr(settings, "AGENT_MODEL", "qwen2.5:7b")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "Ollama running, model qwen2.5:7b found" in result.output


def test_doctor_backend_unreachable_is_reported_not_raised(monkeypatch):
    def fake_get(url, *a, **k):
        raise httpx.ConnectError("boom")

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "boom" in result.output
