from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict

# Config lives in the user's home dir — works whether installed via pip or cloned.
# The repo's runner/.env (if present) is loaded first so a clone with that file
# works out of the box; ~/.wso2-runner/.env (written by `wso2-runner configure`)
# is loaded after and overrides it.
CONFIG_DIR = Path.home() / ".wso2-runner"
CONFIG_FILE = CONFIG_DIR / ".env"
_REPO_ENV_FILE = Path(__file__).resolve().parent.parent / ".env"


class RunnerSettings(BaseSettings):
    # Cloud backend to poll
    CLOUD_URL: str = "http://localhost:8000"
    # Used only as a login_hint to pre-fill the Asgardeo sign-in page —
    # identity itself comes from the Asgardeo login, not this value.
    USER_EMAIL: str = ""

    # Asgardeo tenant/org — same tenant as the web frontend and backend.
    # Required, no default: a runner that forgets to set it should fail loudly
    # at startup rather than silently authenticate against the wrong tenant.
    ASGARDEO_ORG: str
    # Client ID of the Runner's own Asgardeo "Native Application" (public
    # client, PKCE, no secret) — separate from the web frontend's client ID.
    # Set this after registering it in the Asgardeo console (see setup docs).
    ASGARDEO_CLIENT_ID: str = ""

    # Poll interval in seconds
    POLL_INTERVAL: float = 2.0

    # LLM provider — same values as the backend
    AGENT_PROVIDER: str = ""
    AGENT_MODEL: str = ""

    ANTHROPIC_API_KEY: str = ""
    # Optional — set only when using a non-native Anthropic-compatible endpoint,
    # e.g. Claude hosted via Azure AI Foundry. Leave unset to use Anthropic's
    # own api.anthropic.com as normal.
    ANTHROPIC_BASE_URL: str = ""

    GEMINI_API_KEY: str = ""

    AZURE_OPENAI_API_KEY: str = ""
    AZURE_OPENAI_ENDPOINT: str = ""
    AZURE_OPENAI_DEPLOYMENT: str = ""
    AZURE_OPENAI_API_VERSION: str = "2024-10-21"

    BROWSER_CHANNEL: str = "chrome"

    # Which monitor MSS captures for compliance screenshots.
    # 1 = primary/laptop screen, 2 = external monitor (if connected).
    SCREENSHOT_MONITOR: int = 1

    model_config = SettingsConfigDict(
        env_file=(str(_REPO_ENV_FILE), str(CONFIG_FILE)),
        extra="ignore",
    )


settings = RunnerSettings()
