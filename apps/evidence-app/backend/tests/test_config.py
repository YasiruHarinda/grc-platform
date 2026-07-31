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

    settings = Settings(_env_file=None)

    assert settings.ASGARDEO_ORG == "test-org"
