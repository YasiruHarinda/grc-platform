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
"""
import sys
import types

import httpx
import pytest
from typer.testing import CliRunner

import wso2_runner.cli as cli
import wso2_runner.config as config_mod
from wso2_runner import oauth
from wso2_runner.config import settings

runner = CliRunner()


@pytest.fixture
def fake_run_forever(monkeypatch):
    """Import wso2_runner.loop (faking out browser_use via a stub agent
    module) and replace its run_forever with a recorder that returns
    immediately, patched at the site `start` actually imports it from.

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
    return calls


@pytest.fixture(autouse=True)
def _restore_agent_provider():
    """`settings` is a process-wide singleton `cli.py` reaches into by
    importing it fresh each call; tests that mutate AGENT_PROVIDER must not
    leak that mutation into other tests in this file or other test files."""
    original = settings.AGENT_PROVIDER
    yield
    settings.AGENT_PROVIDER = original


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


def test_configure_azure_provider_writes_endpoint_and_deployment(monkeypatch, tmp_path):
    cfg_dir = tmp_path / ".wso2-runner"
    cfg_file = cfg_dir / ".env"
    monkeypatch.setattr(config_mod, "CONFIG_DIR", cfg_dir)
    monkeypatch.setattr(config_mod, "CONFIG_FILE", cfg_file)

    # provider=azure (default, just press enter), model=<default>,
    # api key, endpoint, deployment=<default>, monitor=<default>
    input_text = "\n\nsk-azure-key\nhttps://myorg.openai.azure.com\n\n1\n"

    result = runner.invoke(cli.app, ["configure"], input=input_text)

    assert result.exit_code == 0
    content = cfg_file.read_text()
    assert "AGENT_PROVIDER=azure" in content
    assert "AZURE_OPENAI_API_KEY=sk-azure-key" in content
    assert "AZURE_OPENAI_ENDPOINT=https://myorg.openai.azure.com" in content


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


def test_doctor_reports_missing_azure_key(monkeypatch):
    def fake_get(url, *a, **k):
        return _FakeResponse({"status": "ok"})

    monkeypatch.setattr(httpx, "get", fake_get)
    monkeypatch.setattr(settings, "ASGARDEO_CLIENT_ID", "")
    monkeypatch.setattr(settings, "AGENT_PROVIDER", "azure")
    monkeypatch.setattr(settings, "AZURE_OPENAI_API_KEY", "")

    result = runner.invoke(cli.app, ["doctor", "--server", "http://cloud.test"])

    assert result.exit_code == 0
    assert "AZURE_OPENAI_API_KEY is not set" in result.output


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
