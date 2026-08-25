"""Unit tests for `wso2_runner.config.RunnerSettings`.

These pin the runner's configuration contract: the documented defaults, that
an environment variable overrides its default, that numeric fields are
coerced from the strings the environment always hands over, and that an
unknown setting is ignored rather than crashing the runner at startup.

Every construction passes `_env_file=None` so the tests are isolated from
whatever `runner/.env` or `~/.wso2-runner/.env` happens to exist on the
developer's machine; only the process environment (which monkeypatch
controls) is read.
"""
import os
import subprocess
import sys
from pathlib import Path

import pytest
from pydantic import ValidationError

from wso2_runner.config import RunnerSettings

# Root of the runner package (one level up from this tests/ directory) —
# used as the cwd for the subprocess test below, so a plain `import
# wso2_runner...` resolves the same way it does for every other test here.
_RUNNER_ROOT = Path(__file__).resolve().parent.parent

# Fields the default-test asserts on; cleared from the process env so a stray
# export on the machine cannot mask the real default.
_ASSERTED_DEFAULTS = [
    "CLOUD_URL",
    "USER_EMAIL",
    "ASGARDEO_CLIENT_ID",
    "POLL_INTERVAL",
    "SCREENSHOT_MONITOR",
]


def test_defaults_when_no_env_or_file(monkeypatch):
    for key in _ASSERTED_DEFAULTS:
        monkeypatch.delenv(key, raising=False)

    s = RunnerSettings(_env_file=None)

    assert s.CLOUD_URL == "http://localhost:8000"
    assert s.USER_EMAIL == ""
    assert s.ASGARDEO_CLIENT_ID == ""
    assert s.POLL_INTERVAL == 2.0
    assert s.SCREENSHOT_MONITOR == 1


def test_asgardeo_org_defaults_to_empty_string(monkeypatch):
    # Used to be required with no default, on the theory that a runner
    # forgetting to set it should fail loudly rather than "silently
    # authenticate against the wrong tenant" — but required-ness never
    # protected against a WRONG org, only a MISSING one, and an empty org
    # can't authenticate against any tenant either: it just builds an
    # invalid URL and fails. Worse, the old required field made
    # *importing this module* raise before the CLI wizard that fixes a
    # missing value could even start (see cli.py's `configure`). A missing
    # value is now caught, and reported in plain language, by `start` and
    # `doctor` instead.
    monkeypatch.delenv("ASGARDEO_ORG", raising=False)

    s = RunnerSettings(_env_file=None)

    assert s.ASGARDEO_ORG == ""


def test_import_never_raises_on_a_machine_with_no_config(tmp_path):
    """Reproduces the original crash end to end, in a real subprocess.

    This test's own process already has `wso2_runner.config` imported (and
    its module-level `settings = RunnerSettings()` already evaluated)
    before this test body ever runs, so nothing in-process can re-trigger
    that line under the missing-value condition. A subprocess, with HOME
    pointed at an empty directory (so no ~/.wso2-runner/.env exists either),
    is the only way to prove a genuinely fresh machine can import this
    module at all.
    """
    # PYTHONPATH is set explicitly to this process's own sys.path: Python
    # resolves "user site-packages" (where pydantic-settings etc. live on
    # this machine) from $HOME, so simply pointing HOME at an empty tmp_path
    # would otherwise also hide this process's real dependencies, and the
    # subprocess would fail for a reason that has nothing to do with the bug
    # under test.
    env = {
        **os.environ,
        "HOME": str(tmp_path),
        "PYTHONPATH": os.pathsep.join(p for p in sys.path if p),
    }
    env.pop("ASGARDEO_ORG", None)

    result = subprocess.run(
        [sys.executable, "-c", "import wso2_runner.config"],
        cwd=_RUNNER_ROOT,
        env=env,
        capture_output=True,
        text=True,
    )

    assert result.returncode == 0, result.stderr


def test_env_var_overrides_default(monkeypatch):
    monkeypatch.setenv("CLOUD_URL", "https://cloud.example.com")
    monkeypatch.setenv("ASGARDEO_ORG", "acme")

    s = RunnerSettings(_env_file=None)

    assert s.CLOUD_URL == "https://cloud.example.com"
    assert s.ASGARDEO_ORG == "acme"


def test_numeric_fields_are_coerced_from_env_strings(monkeypatch):
    # The environment only ever yields strings; the float and int fields must
    # come back as real numbers, not strings.
    monkeypatch.setenv("POLL_INTERVAL", "5.5")
    monkeypatch.setenv("SCREENSHOT_MONITOR", "2")

    s = RunnerSettings(_env_file=None)

    assert s.POLL_INTERVAL == 5.5
    assert isinstance(s.POLL_INTERVAL, float)
    assert s.SCREENSHOT_MONITOR == 2
    assert isinstance(s.SCREENSHOT_MONITOR, int)


def test_unknown_setting_is_ignored_not_fatal():
    # `extra = "ignore"` means an unexpected input must not raise, so a
    # leftover or misspelled setting can never stop the runner from starting.
    s = RunnerSettings(_env_file=None, SOME_UNEXPECTED_SETTING="whatever")

    assert not hasattr(s, "SOME_UNEXPECTED_SETTING")
    # A real field still resolves normally alongside the ignored one.
    assert s.CLOUD_URL


# --- Azure auth mode -------------------------------------------------------
#
# The mode used to be a plain string compared against "api_key", so every
# other value, including a misspelling, silently meant entra. Someone rolling
# back to the key path with "apikey" stayed on entra and got a sign-in error
# that said nothing about the real cause. Typing it turns that into a
# start-up error naming both valid values.


def test_azure_auth_mode_defaults_to_entra(monkeypatch):
    monkeypatch.delenv("AZURE_OPENAI_AUTH_MODE", raising=False)

    settings = RunnerSettings(_env_file=None)

    assert settings.AZURE_OPENAI_AUTH_MODE == "entra"


@pytest.mark.parametrize("mode", ["entra", "api_key"])
def test_azure_auth_mode_accepts_both_documented_values(monkeypatch, mode):
    monkeypatch.setenv("AZURE_OPENAI_AUTH_MODE", mode)

    assert RunnerSettings(_env_file=None).AZURE_OPENAI_AUTH_MODE == mode


@pytest.mark.parametrize("typo", ["apikey", "api-key", "API_KEY", "Entra", ""])
def test_azure_auth_mode_rejects_anything_else_and_names_the_valid_values(monkeypatch, typo):
    monkeypatch.setenv("AZURE_OPENAI_AUTH_MODE", typo)

    with pytest.raises(ValidationError) as excinfo:
        RunnerSettings(_env_file=None)

    message = str(excinfo.value)
    assert "AZURE_OPENAI_AUTH_MODE" in message
    # The engineer has to be able to fix it from the error alone.
    assert "entra" in message and "api_key" in message
