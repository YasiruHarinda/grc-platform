from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    DATABASE_URL: str

    # Asgardeo tenant/org — used to build the UserInfo endpoint that validates
    # every request's Bearer token. Required: no default, so a deployment
    # that forgets to set it fails loudly at startup instead of silently
    # validating logins against the wrong tenant.
    ASGARDEO_ORG: str

    # The two Asgardeo client applications that may call this API: the web
    # frontend and the local Runner. Both feed the token audience allow-list,
    # but they are NOT interchangeable — only the web app one gates admin, so
    # they stay two named settings rather than one comma-separated list.
    #
    # Why that matters: Asgardeo's role claim is organisation-wide, not
    # per-application, so a Runner token carries the web-app role names too.
    # The audience is the only claim that says which application a token was
    # actually issued to, and therefore the only thing that can stop a Runner
    # token from granting admin.
    ASGARDEO_WEBAPP_CLIENT_ID: str
    ASGARDEO_RUNNER_CLIENT_ID: str

    # Asgardeo role names granting each role in this application. Configuration
    # rather than constants because role names are environment-specific (staging
    # and production carry different suffixes).
    ASGARDEO_ADMIN_ROLE: str
    ASGARDEO_ENGINEER_ROLE: str

    # Azure Blob Storage for evidence files. The connection string is required;
    # the backend has no local-filesystem fallback.
    AZURE_STORAGE_CONNECTION_STRING: str
    AZURE_STORAGE_CONTAINER: str = "uploads"

    # Comma-separated CORS allow-list for the web frontend.
    CORS_ORIGINS: str = "http://localhost:5173,http://localhost:5174"

    # SQLAlchemy connection pool size, per process (this is a hard cap — see
    # app/database.py, which sets max_overflow=0). Each pod runs its own
    # process with its own pool, so DB_POOL_SIZE x max_pod_count must stay
    # comfortably under Postgres's own max_connections.
    DB_POOL_SIZE: int = 30

    @model_validator(mode="after")
    def _client_ids_must_be_set_and_distinct(self):
        """Refuse to start unless the two client IDs are present and differ.

        Being required is not enough: an empty string satisfies a required
        `str`, and two IDs that are merely *equal* satisfy both. Either case
        removes the audience check silently, which is the one thing standing
        between a Runner token and admin — Asgardeo's role claim is
        organisation-wide, so a Runner token already carries the web app's
        role names, and only the audience says which application issued it.

        Both failures are configuration mistakes that look healthy at
        runtime, which is why they are caught here instead. Blank IDs would
        start cleanly and then lock every user out, because no real token
        carries an empty audience. Identical IDs would start cleanly and
        quietly hand admin to anyone running the Runner. The first is
        alarming and obvious; the second is neither, and is the reason this
        exists.

        They are checked together because two blanks are also two equal
        values, and being told "the client IDs must differ" when both are
        simply unset would send an operator looking in the wrong place.
        """
        if not self.ASGARDEO_WEBAPP_CLIENT_ID.strip() or not self.ASGARDEO_RUNNER_CLIENT_ID.strip():
            raise ValueError(
                "ASGARDEO_WEBAPP_CLIENT_ID and ASGARDEO_RUNNER_CLIENT_ID must both "
                "be set. They are the token audience allow-list; an empty one "
                "refuses every user."
            )
        if self.ASGARDEO_WEBAPP_CLIENT_ID == self.ASGARDEO_RUNNER_CLIENT_ID:
            raise ValueError(
                "ASGARDEO_WEBAPP_CLIENT_ID and ASGARDEO_RUNNER_CLIENT_ID must "
                "differ — they are two separate Asgardeo applications. Setting "
                "them to the same value removes the audience check that stops a "
                "Runner token being granted admin."
            )
        return self


settings = Settings()
