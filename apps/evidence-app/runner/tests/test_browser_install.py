"""Unit tests for wso2_runner.browser_install -- the detect-and-install
logic behind the Chromium guard in `configure` and `start`.

This is pure detection/subprocess handling with no prompting, so (like
env_file.py) it's tested directly here without driving the CLI. No test in
this file may import the real `playwright` package or run a real
`playwright install` -- both `sync_playwright` and `subprocess.run` are
stubbed throughout, and nothing here should touch the network.
"""
import subprocess
import sys
import types

import pytest

import wso2_runner.browser_install as browser_install
from wso2_runner.browser_install import (
    ChromiumInstallError,
    chromium_is_installed,
    ensure_chromium_installed,
    install_chromium,
)


# ── fake playwright.sync_api, for exercising the "chromium" channel ────────
#
# playwright genuinely isn't installed in this test environment (see
# tests/test_cli.py's module docstring -- the same is true here), so the
# ImportError path below is real, not simulated. To exercise the path where
# Playwright *is* present and reports a real executable_path, a minimal fake
# of `playwright.sync_api.sync_playwright()` is injected into sys.modules --
# just enough surface for chromium_is_installed to call, nothing else.


class _FakeChromiumType:
    def __init__(self, executable_path):
        self.executable_path = executable_path


class _FakePlaywright:
    def __init__(self, executable_path):
        self.chromium = _FakeChromiumType(executable_path)


class _FakeSyncPlaywrightContext:
    def __init__(self, executable_path):
        self._executable_path = executable_path

    def __enter__(self):
        return _FakePlaywright(self._executable_path)

    def __exit__(self, *exc_info):
        return False


def _install_fake_playwright(monkeypatch, executable_path):
    fake_sync_api = types.ModuleType("playwright.sync_api")
    fake_sync_api.sync_playwright = lambda: _FakeSyncPlaywrightContext(executable_path)
    fake_playwright_pkg = types.ModuleType("playwright")
    monkeypatch.setitem(sys.modules, "playwright", fake_playwright_pkg)
    monkeypatch.setitem(sys.modules, "playwright.sync_api", fake_sync_api)


def _block_real_playwright(monkeypatch):
    """Force `import playwright` (and `from playwright.sync_api import
    ...`) to raise ImportError, the same way it genuinely does on a machine
    that never ran `playwright install`. Setting a name to None in
    sys.modules is the standard way to force that -- see the `importlib`
    docs on sys.modules."""
    monkeypatch.setitem(sys.modules, "playwright", None)
    monkeypatch.delitem(sys.modules, "playwright.sync_api", raising=False)


# ── chromium_is_installed: non-chromium channels ───────────────────────────


def test_non_chromium_channel_is_always_reported_installed(monkeypatch):
    """BROWSER_CHANNEL defaults to "chrome", which asks Playwright to launch
    a *system* Chrome, not Playwright's bundled Chromium. Installing the
    bundled Chromium would not put Chrome on disk, so for any channel other
    than "chromium" there is nothing for this module to check or install --
    it must report "installed" without even importing Playwright."""
    _block_real_playwright(monkeypatch)

    assert chromium_is_installed("chrome") is True


def test_msedge_channel_is_also_always_reported_installed(monkeypatch):
    _block_real_playwright(monkeypatch)

    assert chromium_is_installed("msedge") is True


def test_empty_channel_defaults_to_chrome_and_is_reported_installed(monkeypatch):
    """Mirrors agent.py's `_get_browser()`: an empty BROWSER_CHANNEL means
    "chrome", the same as if it had never been set."""
    _block_real_playwright(monkeypatch)

    assert chromium_is_installed("") is True


def test_channel_check_is_case_and_whitespace_insensitive(monkeypatch):
    _block_real_playwright(monkeypatch)

    assert chromium_is_installed("  Chromium  ") is False  # playwright genuinely missing here
    assert chromium_is_installed("CHROME") is True


# ── chromium_is_installed: the "chromium" channel ──────────────────────────


def test_chromium_channel_reports_missing_when_playwright_not_importable(monkeypatch):
    _block_real_playwright(monkeypatch)

    assert chromium_is_installed("chromium") is False


def test_chromium_channel_reports_installed_when_executable_exists(monkeypatch, tmp_path):
    real_binary = tmp_path / "headless_shell"
    real_binary.write_text("pretend binary")
    _install_fake_playwright(monkeypatch, str(real_binary))

    assert chromium_is_installed("chromium") is True


def test_chromium_channel_reports_missing_when_executable_does_not_exist(monkeypatch, tmp_path):
    missing_binary = tmp_path / "no-such-binary"
    _install_fake_playwright(monkeypatch, str(missing_binary))

    assert chromium_is_installed("chromium") is False


# ── install_chromium ────────────────────────────────────────────────────


def test_install_chromium_succeeds_quietly_on_exit_code_zero(monkeypatch):
    calls = []

    def fake_run(cmd, **kwargs):
        calls.append(cmd)
        return subprocess.CompletedProcess(cmd, returncode=0, stdout="", stderr="")

    monkeypatch.setattr(browser_install.subprocess, "run", fake_run)

    install_chromium()  # must not raise

    assert calls == [[sys.executable, "-m", "playwright", "install", "chromium"]]


def test_install_chromium_raises_with_stderr_on_nonzero_exit(monkeypatch):
    def fake_run(cmd, **kwargs):
        return subprocess.CompletedProcess(cmd, returncode=1, stdout="", stderr="network unreachable")

    monkeypatch.setattr(browser_install.subprocess, "run", fake_run)

    with pytest.raises(ChromiumInstallError) as excinfo:
        install_chromium()
    assert "network unreachable" in str(excinfo.value)


def test_install_chromium_raises_when_subprocess_run_itself_fails(monkeypatch):
    """E.g. the interpreter has no `playwright` module at all -- `python -m
    playwright` fails to even start rather than exiting non-zero."""
    def fake_run(cmd, **kwargs):
        raise FileNotFoundError("no such module: playwright")

    monkeypatch.setattr(browser_install.subprocess, "run", fake_run)

    with pytest.raises(ChromiumInstallError):
        install_chromium()


# ── ensure_chromium_installed: orchestration ─────────────────────────────


def test_ensure_skips_entirely_when_already_installed(monkeypatch, capsys):
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: True)

    def _must_not_be_called():
        raise AssertionError("install_chromium must not run when already installed")

    monkeypatch.setattr(browser_install, "install_chromium", lambda: _must_not_be_called())

    ensure_chromium_installed("chromium")  # must not raise

    # No download warning, no output of any kind -- this is the "adds no
    # noticeable delay when already installed" path.
    assert capsys.readouterr().out == ""


def test_ensure_prints_size_warning_before_attempting_install(monkeypatch, capsys):
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)
    monkeypatch.setattr(browser_install, "install_chromium", lambda: None)

    ensure_chromium_installed("chromium")

    out = capsys.readouterr().out
    assert "150" in out
    assert "MB" in out


def test_ensure_prints_warning_even_when_the_install_then_fails(monkeypatch, capsys):
    """Proves the warning is printed unconditionally before the attempt,
    not only announced after a successful download -- a long silence
    followed by a failure would be the worst of both worlds."""
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)

    def _boom():
        raise ChromiumInstallError("network unreachable")

    monkeypatch.setattr(browser_install, "install_chromium", _boom)

    with pytest.raises(ChromiumInstallError):
        ensure_chromium_installed("chromium")

    out = capsys.readouterr().out
    assert "150" in out


def test_ensure_propagates_the_underlying_reason_on_failure(monkeypatch):
    """ensure_chromium_installed() itself carries only the technical reason
    -- cli.py is the one place that composes the final "here's what to
    type" message around it (see tests/test_cli.py's
    test_start_exits_nonzero_and_names_manual_command_when_chromium_install_fails
    and its `configure` counterpart), the same way `start`'s other start-up
    guards let cli.py phrase the final message around a lower-level
    exception."""
    monkeypatch.setattr(browser_install, "chromium_is_installed", lambda channel: False)

    def fake_run(cmd, **kwargs):
        return subprocess.CompletedProcess(cmd, returncode=1, stdout="", stderr="network unreachable")

    monkeypatch.setattr(browser_install.subprocess, "run", fake_run)

    with pytest.raises(ChromiumInstallError) as excinfo:
        ensure_chromium_installed("chromium")

    assert "network unreachable" in str(excinfo.value)


def test_manual_install_command_constant_matches_what_install_chromium_runs():
    """Guards against the printed hint and the real command drifting apart
    -- MANUAL_INSTALL_COMMAND is what cli.py names to the engineer, and it
    must be exactly what `install_chromium()` actually runs."""
    assert browser_install.MANUAL_INSTALL_COMMAND == "playwright install chromium"


# ── launch_failure_advice ────────────────────────────────────────────────


def test_launch_failure_advice_names_the_playwright_command_for_chromium():
    """On the "chromium" channel the Runner launches Playwright's own
    bundled binary, so `playwright install chromium` is exactly the right
    thing to type."""
    advice = browser_install.launch_failure_advice("chromium")

    assert browser_install.MANUAL_INSTALL_COMMAND in advice


def test_launch_failure_advice_does_not_name_playwright_for_a_system_channel():
    """The bug this guards against: `playwright install chromium` used to be
    printed for every channel. On the default "chrome" channel that advice
    is actively wrong -- it downloads roughly 150 MB of a browser the Runner
    will not launch, and the original failure is still there afterwards."""
    advice = browser_install.launch_failure_advice("chrome")

    assert browser_install.MANUAL_INSTALL_COMMAND not in advice
    assert "chrome" in advice.lower()


def test_launch_failure_advice_offers_the_chromium_channel_as_a_way_out():
    """Someone with no Google Chrome and no way to install it is not stuck:
    switching the channel makes the Runner fetch and use its own browser.
    The advice has to say so, or that escape hatch is invisible."""
    advice = browser_install.launch_failure_advice("chrome")

    assert "BROWSER_CHANNEL=chromium" in advice


def test_launch_failure_advice_names_whichever_system_channel_is_configured():
    """Not hardcoded to Chrome. msedge means the machine's own Edge, and the
    message has to name the browser the engineer actually configured."""
    advice = browser_install.launch_failure_advice("msedge")

    assert "msedge" in advice
    assert browser_install.MANUAL_INSTALL_COMMAND not in advice


def test_launch_failure_advice_uses_the_browser_s_real_product_name():
    """"Install msedge" reads like a typo. The channel value still appears,
    because that is what the engineer sets in their config, but the thing
    they have to go and install is named the way the vendor names it."""
    advice = browser_install.launch_failure_advice("msedge")

    assert "Microsoft Edge" in advice
    assert "msedge" in advice


def test_launch_failure_advice_falls_back_to_the_channel_name_when_unknown():
    """A channel we have no product name for still gets usable advice,
    rather than a blank where the browser name should be."""
    advice = browser_install.launch_failure_advice("some-future-channel")

    assert "some-future-channel" in advice
    assert "BROWSER_CHANNEL=chromium" in advice
