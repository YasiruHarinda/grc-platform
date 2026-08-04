"""
Direct settings-construction tests for `app.config.Settings`.

`ASGARDEO_ORG` must be a required setting with no default — a deployment
that forgets to set it should fail loudly at startup instead of silently
validating logins against the wrong tenant (see issue #8).

`conftest.py` puts `ASGARDEO_ORG` in the process environment (along with
`DATABASE_URL` and `AZURE_STORAGE_CONNECTION_STRING`) before `app.main` is
ever imported, so the "missing" case below must explicitly remove it from
the environment for that one test, and construct `Settings` with
`_env_file=None` so a repo-root `.env` file (which is gitignored and may
exist on a developer machine) can't supply it either.
"""
import pytest
from pydantic import ValidationError

from app.config import Settings


REQUIRED_ASGARDEO_SETTINGS = [
    "ASGARDEO_ORG",
    "ASGARDEO_WEBAPP_CLIENT_ID",
    "ASGARDEO_RUNNER_CLIENT_ID",
    "ASGARDEO_ADMIN_ROLE",
    "ASGARDEO_ENGINEER_ROLE",
]


@pytest.mark.parametrize("name", REQUIRED_ASGARDEO_SETTINGS)
def test_settings_raises_when_an_asgardeo_setting_is_missing(monkeypatch, name):
    """Every Asgardeo setting is required, one test per setting.

    The client IDs are the audience allow-list and the role names are the
    authorisation gate, so a deployment that forgets one must not start. An
    empty allow-list in particular would accept a token minted for any other
    application in the organisation — an authentication bypass — and it would
    do so silently, which is exactly the failure a default would hide.
    """
    monkeypatch.delenv(name, raising=False)

    with pytest.raises(ValidationError, match=name):
        Settings(_env_file=None)


def test_settings_loads_when_asgardeo_org_is_present(monkeypatch):
    monkeypatch.setenv("ASGARDEO_ORG", "test-org")
    # Spelled out rather than inherited from conftest, because the validator
    # below makes this test depend on the two IDs differing. Left implicit, a
    # future edit to conftest's values could fail this test for a reason that
    # has nothing to do with what it checks.
    monkeypatch.setenv("ASGARDEO_WEBAPP_CLIENT_ID", "webapp-client-id")
    monkeypatch.setenv("ASGARDEO_RUNNER_CLIENT_ID", "runner-client-id")

    settings = Settings(_env_file=None)

    assert settings.ASGARDEO_ORG == "test-org"


@pytest.mark.parametrize(
    "webapp, runner",
    [
        ("same-client-id", "same-client-id"),  # the dangerous one
        ("", ""),                               # both unset
        ("", "runner-client-id"),               # web app unset
        ("webapp-client-id", ""),               # runner unset
        ("   ", "runner-client-id"),            # whitespace is not a value
    ],
)
def test_settings_rejects_client_ids_that_are_blank_or_identical(monkeypatch, webapp, runner):
    """Being a required `str` is not enough to make these two safe.

    An empty string satisfies a required `str`, and two equal strings satisfy
    both settings, so neither case is caught by the required-setting tests
    above. Each removes the audience check — the only thing that stops a
    Runner token being granted admin, since Asgardeo's role claim is
    organisation-wide and a Runner token already carries the web app's role
    names.

    Identical IDs are the case worth the test. Blank IDs fail loudly at
    runtime, because no real token carries an empty audience, so everyone is
    locked out and somebody investigates within minutes. Identical IDs fail
    silently in the dangerous direction: the app works, everyone logs in, and
    anyone running the Runner quietly holds admin. `.env.example` ships both
    fields blank, so the blank case is the likelier mistake and the identical
    case is the more expensive one.
    """
    monkeypatch.setenv("ASGARDEO_ORG", "test-org")
    monkeypatch.setenv("ASGARDEO_WEBAPP_CLIENT_ID", webapp)
    monkeypatch.setenv("ASGARDEO_RUNNER_CLIENT_ID", runner)

    with pytest.raises(ValidationError, match="CLIENT_ID"):
        Settings(_env_file=None)
