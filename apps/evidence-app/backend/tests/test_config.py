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


def test_settings_raises_when_asgardeo_org_is_missing(monkeypatch):
    monkeypatch.delenv("ASGARDEO_ORG", raising=False)

    with pytest.raises(ValidationError, match="ASGARDEO_ORG"):
        Settings(_env_file=None)


def test_settings_loads_when_asgardeo_org_is_present(monkeypatch):
    monkeypatch.setenv("ASGARDEO_ORG", "test-org")

    settings = Settings(_env_file=None)

    assert settings.ASGARDEO_ORG == "test-org"
