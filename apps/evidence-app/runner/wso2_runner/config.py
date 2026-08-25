from pathlib import Path
from typing import Literal

from pydantic_settings import BaseSettings, SettingsConfigDict

# The two ways the Runner can authorise Azure OpenAI calls. Named here and
# imported everywhere else, rather than each caller spelling the string out:
# a bare "api_key" repeated across modules is the kind of thing that survives
# a rename in one file and silently changes meaning in another.
AZURE_AUTH_ENTRA = "entra"
AZURE_AUTH_API_KEY = "api_key"
AzureAuthMode = Literal["entra", "api_key"]

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
    # Used to be required with no default, on the theory that a runner
    # forgetting to set it should fail loudly rather than "silently
    # authenticate against the wrong tenant". That reasoning doesn't hold up:
    # required-ness never protected against a WRONG org, only a MISSING one,
    # and an empty org can't authenticate against any tenant either — it just
    # builds an invalid URL and fails. Worse, being required here made
    # *importing this module* raise before `wso2-runner configure` (the
    # wizard meant to fix a missing config) could even start, because cli.py
    # imports CONFIG_DIR/CONFIG_FILE from this module and that import runs
    # the `settings = RunnerSettings()` line below. A missing value is now
    # caught, and reported in plain language, by `start` and `doctor`
    # instead — see cli.py.
    ASGARDEO_ORG: str = ""
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
    # "entra" (default): each engineer's own Azure CLI session authorises
    # calls, no credential written to disk. "api_key": fall back to
    # AZURE_OPENAI_API_KEY above — reachable only by setting this
    # explicitly, never as an implicit fallback.
    # Typed, not a bare str, so a misspelling is rejected at start-up with a
    # message naming the valid values. Left as a free string, anything that
    # wasn't exactly "api_key" quietly meant entra, so an engineer rolling
    # back with "apikey" stayed on entra and got a puzzling sign-in error
    # instead of being told their setting was wrong.
    AZURE_OPENAI_AUTH_MODE: AzureAuthMode = AZURE_AUTH_ENTRA
    # Required when AZURE_OPENAI_AUTH_MODE is "entra" — pins the credential
    # to one tenant so switching Azure tenants/subscriptions for other work
    # doesn't intermittently break LLM access. Not needed in "api_key" mode.
    AZURE_TENANT_ID: str = ""

    BROWSER_CHANNEL: str = "chrome"

    # Which monitor MSS captures for compliance screenshots.
    # 1 = primary/laptop screen, 2 = external monitor (if connected).
    SCREENSHOT_MONITOR: int = 1

    model_config = SettingsConfigDict(
        env_file=(str(_REPO_ENV_FILE), str(CONFIG_FILE)),
        extra="ignore",
    )


settings = RunnerSettings()
