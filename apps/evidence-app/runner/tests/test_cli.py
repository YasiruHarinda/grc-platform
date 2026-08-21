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
import stat
import subprocess
import sys
import types
from pathlib import Path

import httpx
import pytest
from PIL import Image
from typer.testing import CliRunner

import wso2_runner.browser_install as browser_install
import wso2_runner.capture_check as capture_check
import wso2_runner.cli as cli
import wso2_runner.config as config_mod
from wso2_runner import oauth
from wso2_runner.azure_credential import ClientAuthenticationError, CredentialUnavailableError
from wso2_runner.browser_install import ChromiumInstallError
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

    # email, provider=anthropic, model=<accept default>, api key, monitor=<accept default>
    input_text = "someone@wso2.com\nanthropic\n\nsk-test-key\n1\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    assert cfg_file.exists()
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=anthropic" in content
    assert "ANTHROPIC_API_KEY=sk-test-key" in content
    assert "SCREENSHOT_MONITOR=1" in content
    assert "USER_EMAIL=someone@wso2.com" in content


def test_configure_azure_provider_writes_endpoint_deployment_and_tenant(monkeypatch, tmp_path):
    """Azure setup no longer hands out a shared API key — see ticket #95.
    The wizard writes endpoint, deployment and tenant, and points the
    engineer at `az login` instead of prompting for a secret."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # email, provider=azure (default, just press enter), model=<default>,
    # endpoint, deployment=<default>, tenant ID, monitor=<default>
    input_text = "someone@wso2.com\n\n\nhttps://myorg.openai.azure.com\n\nsome-tenant-id\n1\n"

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

    # email, provider=ollama, model=<default>, monitor=<default>
    input_text = "someone@wso2.com\nollama\n\n1\n"

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

    # email, provider=gemini, model=<default>, api key, monitor=2 (triggers
    # the "drag the agent's Chrome window" tip branch)
    input_text = "someone@wso2.com\ngemini\n\nsk-gemini-key\n2\n"

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

    # email, provider=ollama, model=<default>, monitor=<default> — needs no API key.
    result = subprocess.run(
        [sys.executable, "-c", "import sys; sys.argv = ['wso2-runner', 'configure']; from wso2_runner.cli import app; app()"],
        cwd=_RUNNER_ROOT,
        env=env,
        input="someone@wso2.com\nollama\n\n1\n",
        capture_output=True,
        text=True,
    )

    assert result.returncode == 0, result.stderr
    config_file = tmp_path / ".wso2-runner" / ".env"
    assert config_file.exists()
    assert "AGENT_PROVIDER=ollama" in config_file.read_text()


# ── configure: in-place update, not replace ─────────────────────────────
#
# The bug this whole ticket exists for: `configure` used to build its
# output from scratch and call CONFIG_FILE.write_text(...), which replaces
# the entire file. CLOUD_URL, ASGARDEO_ORG, ASGARDEO_CLIENT_ID and
# USER_EMAIL are never in the list it builds, so a second run of a
# documented command silently deleted them, with no warning and no backup.


def test_configure_twice_leaves_hand_set_fields_and_email_intact(monkeypatch, tmp_path):
    """Runs the wizard twice against a file that already has settings the
    wizard itself never asks about (CLOUD_URL, ASGARDEO_ORG,
    ASGARDEO_CLIENT_ID — set by hand per the setup docs) plus a USER_EMAIL
    from a previous `configure` run. All four must survive both runs."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text(
        "CLOUD_URL=https://cloud.example.com\n"
        "ASGARDEO_ORG=wso2\n"
        "ASGARDEO_CLIENT_ID=abc123\n"
        "USER_EMAIL=first@wso2.com\n"
    )
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # email=<accept default: first@wso2.com>, provider=ollama, model=<default>, monitor=<default>
    input_text = "\nollama\n\n1\n"

    first = runner.invoke(cli.app, ["configure"], input=input_text)
    assert first.exit_code == 0, first.output

    second = runner.invoke(cli.app, ["configure"], input=input_text)
    assert second.exit_code == 0, second.output

    content = cfg_file.read_text()
    assert "CLOUD_URL=https://cloud.example.com" in content
    assert "ASGARDEO_ORG=wso2" in content
    assert "ASGARDEO_CLIENT_ID=abc123" in content
    assert "USER_EMAIL=first@wso2.com" in content


def test_configure_preserves_a_hand_added_key_it_never_asks_about(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text("MY_HAND_ADDED_NOTE=do-not-touch\n")
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    input_text = "me@wso2.com\nollama\n\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    assert "MY_HAND_ADDED_NOTE=do-not-touch" in cfg_file.read_text()


def test_configure_preserves_comments_and_blank_lines(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text(
        "# personal notes, do not delete\n"
        "\n"
        "AGENT_PROVIDER=azure\n"
        "\n"
        "# end of file\n"
    )
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    input_text = "me@wso2.com\nollama\n\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    lines = cfg_file.read_text().splitlines()
    assert "# personal notes, do not delete" in lines
    assert "# end of file" in lines
    assert lines.count("") >= 2


def test_configure_updates_an_existing_key_in_place_without_duplicating_it(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text("AGENT_PROVIDER=azure\nAGENT_MODEL=old-model\n")
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    input_text = "me@wso2.com\nollama\n\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    content = cfg_file.read_text()
    assert content.count("AGENT_PROVIDER=") == 1
    assert "AGENT_PROVIDER=ollama" in content


def test_configure_sets_file_mode_to_owner_read_write_only(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    input_text = "me@wso2.com\nanthropic\n\nsk-test-key\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    mode = stat.S_IMODE(cfg_file.stat().st_mode)
    assert mode == 0o600


def test_configure_prompts_for_and_writes_user_email(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    input_text = "engineer@wso2.com\nollama\n\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    assert "USER_EMAIL=engineer@wso2.com" in cfg_file.read_text()
    assert "email" in result.output.lower()


# ── configure: --server ──────────────────────────────────────────────────
#
# `configure` never wrote CLOUD_URL, ASGARDEO_ORG or ASGARDEO_CLIENT_ID --
# a new machine was left pointed at the http://localhost:8000 default in
# config.py, which looks configured but polls nothing. `--server <url>`
# fetches the org and client ID from that server's `/api/runner-config` and
# saves all three together. httpx.get is stubbed throughout, exactly as the
# `doctor` tests below stub it -- no test here reaches the network.


def test_configure_with_server_writes_cloud_url_org_and_client_id(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        assert url == "https://cloud.example.com/api/runner-config"
        return _FakeResponse({"asgardeo_org": "wso2", "asgardeo_client_id": "abc-123"})

    monkeypatch.setattr(httpx, "get", fake_get)

    # email, provider=ollama, model=<default>, monitor=<default>
    input_text = "someone@wso2.com\nollama\n\n1\n"
    result = runner.invoke(
        cli.app, ["configure", "--server", "https://cloud.example.com"], input=input_text
    )

    assert result.exit_code == 0, result.output
    content = cfg_file.read_text()
    assert "CLOUD_URL=https://cloud.example.com" in content
    assert "ASGARDEO_ORG=wso2" in content
    assert "ASGARDEO_CLIENT_ID=abc-123" in content


def test_configure_server_values_come_from_response_not_a_stale_file(monkeypatch, tmp_path):
    """The org and client ID written must be whatever the endpoint just
    returned, not whatever happened to already be on disk -- proves the
    wizard doesn't just leave an old value in place under a new CLOUD_URL."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text("ASGARDEO_ORG=stale-org\nASGARDEO_CLIENT_ID=stale-client\n")
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        return _FakeResponse({"asgardeo_org": "fresh-org", "asgardeo_client_id": "fresh-client"})

    monkeypatch.setattr(httpx, "get", fake_get)

    input_text = "someone@wso2.com\nollama\n\n1\n"
    result = runner.invoke(
        cli.app, ["configure", "--server", "https://cloud.example.com"], input=input_text
    )

    assert result.exit_code == 0, result.output
    content = cfg_file.read_text()
    assert "ASGARDEO_ORG=fresh-org" in content
    assert "ASGARDEO_CLIENT_ID=fresh-client" in content
    assert "stale-org" not in content
    assert "stale-client" not in content


def test_configure_server_connection_refused_names_url_and_exits_nonzero(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        raise httpx.ConnectError("refused")

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert "https://cloud.example.com" in result.output


def test_configure_server_404_names_url_and_exits_nonzero(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        return _FakeResponse({}, status_code=404)

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert "https://cloud.example.com" in result.output
    assert "404" in result.output


def test_configure_server_non_json_body_names_url_and_exits_nonzero(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        return _FakeNonJsonResponse()

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert "https://cloud.example.com" in result.output


def test_configure_server_missing_key_names_url_and_exits_nonzero(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        # asgardeo_client_id missing entirely -- a backend too old to have
        # been updated alongside this endpoint might do this.
        return _FakeResponse({"asgardeo_org": "wso2"})

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert "https://cloud.example.com" in result.output


def test_configure_server_failure_writes_no_config_file_at_all(monkeypatch, tmp_path):
    """A failed fetch must exit before any prompt is asked and before
    anything is written -- there is no partial config, and nothing here can
    leave a fresh machine with the http://localhost:8000 default silently
    treated as configured."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        raise httpx.ConnectError("refused")

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert not cfg_file.exists()


def test_configure_server_failure_leaves_an_existing_config_file_untouched(monkeypatch, tmp_path):
    """Same guarantee as above, but against a machine that was already
    configured -- a bad --server must not overwrite CLOUD_URL with a
    partial value or fall back to localhost, it must change nothing."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    original_content = (
        "CLOUD_URL=https://old-cloud.example.com\n"
        "ASGARDEO_ORG=old-org\n"
        "ASGARDEO_CLIENT_ID=old-client\n"
    )
    cfg_file.write_text(original_content)
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        raise httpx.ConnectError("refused")

    monkeypatch.setattr(httpx, "get", fake_get)

    result = runner.invoke(cli.app, ["configure", "--server", "https://cloud.example.com"])

    assert result.exit_code != 0
    assert cfg_file.read_text() == original_content
    assert "localhost" not in cfg_file.read_text()


def test_configure_without_server_leaves_the_three_server_keys_untouched(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    cfg_dir.mkdir()
    cfg_file.write_text(
        "CLOUD_URL=https://cloud.example.com\n"
        "ASGARDEO_ORG=wso2\n"
        "ASGARDEO_CLIENT_ID=abc-123\n"
    )
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    def fake_get(url, *a, **k):
        raise AssertionError("no --server was given, httpx.get should never be called")

    monkeypatch.setattr(httpx, "get", fake_get)

    input_text = "someone@wso2.com\nollama\n\n1\n"
    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0, result.output
    content = cfg_file.read_text()
    assert "CLOUD_URL=https://cloud.example.com" in content
    assert "ASGARDEO_ORG=wso2" in content
    assert "ASGARDEO_CLIENT_ID=abc-123" in content


# ── doctor ───────────────────────────────────────────────────────────────


class _FakeResponse:
    def __init__(self, data, status_code=200):
        self._data = data
        self.status_code = status_code

    def json(self):
        return self._data


class _FakeNonJsonResponse:
    """A 200 whose body isn't JSON -- e.g. an nginx or load balancer error
    page returned instead of the API response. `.json()` raises the same
    way httpx's real `Response.json()` does when the body doesn't parse."""

    def __init__(self):
        self.status_code = 200

    def json(self):
        raise ValueError("not JSON")


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


# ── doctor: blank screen capture check ────────────────────────────────────
#
# `capture_check.capture_test_screenshot` is stubbed in every test below --
# nothing here ever calls the real `mss`, let alone takes a real
# screenshot. The pure blank-or-not analysis (`looks_blank`) and the wording
# of the advice message are covered directly, against synthetic images,
# in tests/test_capture_check.py; these tests only cover doctor's own
# wiring: does it call the capture, does it print the right thing for each
# outcome, and does it still exit zero every time.


def _varied_test_image():
    """A synthetic stand-in for a normal, non-blank screen -- see
    tests/test_capture_check.py for why this shape has high variation."""
    img = Image.new("RGB", (40, 30))
    pixels = img.load()
    for x in range(40):
        for y in range(30):
            pixels[x, y] = (255, 255, 255) if (x // 5 + y // 5) % 2 == 0 else (10, 10, 10)
    return img


def test_doctor_warns_on_blank_capture_but_still_exits_zero(monkeypatch):
    """The key scenario this ticket exists for: a wallpaper-like capture --
    not black, just flat -- must be reported as a warning, and must never
    turn `doctor` itself into a failure."""
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "sk-x")

    wallpaper = Image.new("RGB", (40, 30), (60, 90, 160))
    monkeypatch.setattr(capture_check, "capture_test_screenshot", lambda: wallpaper)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "blank" in result.output.lower()
    # All three details from the ticket must reach the terminal, not just
    # live in the module as an unused constant.
    assert "TERMINAL APP" in result.output
    assert "Cmd+Q" in result.output
    assert "by hand" in result.output
    assert "Wayland" in result.output or "wayland" in result.output


def test_doctor_reports_capture_looks_fine_for_a_varied_screen(monkeypatch):
    """A normal, varied capture must not trip the warning at all -- doctor
    should report success plainly, the same ✓ convention every other
    check uses."""
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "sk-x")
    monkeypatch.setattr(capture_check, "capture_test_screenshot", _varied_test_image)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "blank" not in result.output.lower()
    assert "✓" in result.output


def test_doctor_reports_capture_failure_without_raising(monkeypatch):
    """No display, `mss` missing, whatever the reason -- if the capture
    itself blows up, doctor must say so plainly and keep going, exactly
    like every other check in this command."""
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "anthropic")
    monkeypatch.setattr(settings, "ANTHROPIC_API_KEY", "sk-x")

    def _boom():
        raise RuntimeError("no display available")

    monkeypatch.setattr(capture_check, "capture_test_screenshot", _boom)

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "no display available" in result.output


def test_start_never_touches_the_blank_capture_check(fake_run_forever, monkeypatch):
    """This heuristic is only ever a `doctor` diagnostic. `start` must never
    call it -- a false alarm here must not be able to block a real run."""
    settings.AGENT_PROVIDER = "anthropic"

    def _must_not_run():
        raise AssertionError("start must never take a test screenshot")

    monkeypatch.setattr(capture_check, "capture_test_screenshot", _must_not_run)

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


# ── configure / start: Chromium install guard ────────────────────────────
#
# Nothing here ever imports the real `playwright` package or runs a real
# install -- browser_install.chromium_is_installed and
# browser_install.install_chromium are monkeypatched at the module level in
# every test below. Because cli.py imports `wso2_runner.browser_install`
# lazily, inside the body of `configure`/`start` (matching the rest of this
# file's lazy-import style), patching those two names on the
# `browser_install` module itself is enough -- it doesn't matter how cli.py
# spells the import.
#
# BROWSER_CHANNEL defaults to "chrome" in every other test in this file, and
# is left untouched there -- chromium_is_installed() reports "chrome" as
# always installed without even looking at Playwright (see
# test_browser_install.py), so none of the pre-existing configure/start
# tests above needed to change, or slowed down, when this guard was added.


_CONFIGURE_INPUT = "someone@wso2.com\nanthropic\n\nsk-test-key\n1\n"


def test_configure_installs_chromium_when_missing(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)
    calls = []
    monkeypatch.setattr(browser_install, "install_chromium", lambda: calls.append(1))

    result = runner.invoke(cli.app, ["configure"], input=_CONFIGURE_INPUT)

    assert result.exit_code == 0
    assert calls == [1]
    assert "150" in result.output
    assert cfg_file.exists()


def test_configure_skips_chromium_download_when_already_present(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: True)

    def _must_not_run():
        raise AssertionError("install_chromium must not run when already installed")

    monkeypatch.setattr(browser_install, "install_chromium", lambda: _must_not_run())

    result = runner.invoke(cli.app, ["configure"], input=_CONFIGURE_INPUT)

    assert result.exit_code == 0
    assert "150" not in result.output


def test_configure_reports_manual_command_when_chromium_install_fails_but_still_saves_config(monkeypatch, tmp_path):
    """A failed Chromium download must not look like `configure` itself
    failed -- the config it just wrote is still good, and the engineer
    should still see the "Config saved" / "Next: wso2-runner start"
    messages, plus a plain pointer at the manual command to fix Chromium."""
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)

    def _boom():
        raise ChromiumInstallError("network unreachable")

    monkeypatch.setattr(browser_install, "install_chromium", _boom)

    result = runner.invoke(cli.app, ["configure"], input=_CONFIGURE_INPUT)

    assert result.exit_code == 0
    assert cfg_file.exists()
    assert "playwright install chromium" in result.output
    assert "Config saved" in result.output


def test_start_checks_chromium_and_installs_when_missing(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "anthropic"
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)
    calls = []
    monkeypatch.setattr(browser_install, "install_chromium", lambda: calls.append(1))

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert calls == [1]
    assert "150" in result.output
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_skips_chromium_check_delay_when_already_present(fake_run_forever, monkeypatch):
    settings.AGENT_PROVIDER = "anthropic"
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: True)

    def _must_not_run():
        raise AssertionError("install_chromium must not run when already installed")

    monkeypatch.setattr(browser_install, "install_chromium", lambda: _must_not_run())

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 0
    assert "150" not in result.output
    assert fake_run_forever == [(None, "someone@wso2.com", None)]


def test_start_exits_nonzero_and_names_manual_command_when_chromium_install_fails(fake_run_forever, monkeypatch):
    """The whole point of this guard: a failed Chromium install must be
    reported here, in plain language, and must stop `start` before it ever
    reaches run_forever -- never surfacing later as a confusing exception
    from deep inside a browser launch."""
    settings.AGENT_PROVIDER = "anthropic"
    monkeypatch.setattr(settings, "BROWSER_CHANNEL", "chromium")
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)

    def _boom():
        raise ChromiumInstallError("network unreachable")

    monkeypatch.setattr(browser_install, "install_chromium", _boom)

    result = runner.invoke(cli.app, ["start", "someone@wso2.com"])

    assert result.exit_code == 1
    assert "playwright install chromium" in result.output
    # run_forever must never be reached -- no browser opens on a failed install.
    assert fake_run_forever == []
