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
import pytest
from pydantic import ValidationError

from wso2_runner.config import RunnerSettings

# Fields the default-test asserts on; cleared from the process env so a stray
# export on the machine cannot mask the real default. ASGARDEO_ORG is not here:
# it is required with no default, so it is set (not asserted) so construction
# succeeds, and its own missing-case test lives below.
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
    # Required field, so it must be present for construction to succeed; its
    # value is asserted by test_env_var_overrides_default, not here.
    monkeypatch.setenv("ASGARDEO_ORG", "test-org")

    s = RunnerSettings(_env_file=None)

    assert s.CLOUD_URL == "http://localhost:8000"
    assert s.USER_EMAIL == ""
    assert s.ASGARDEO_CLIENT_ID == ""
    assert s.POLL_INTERVAL == 2.0
    assert s.SCREENSHOT_MONITOR == 1


def test_raises_when_asgardeo_org_is_missing(monkeypatch):
    # Required, no default: a runner that forgets to set the org should fail
    # loudly at startup rather than authenticate against the wrong tenant.
    monkeypatch.delenv("ASGARDEO_ORG", raising=False)

    with pytest.raises(ValidationError, match="ASGARDEO_ORG"):
        RunnerSettings(_env_file=None)


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
