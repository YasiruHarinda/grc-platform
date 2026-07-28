from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    DATABASE_URL: str

    # Asgardeo tenant/org — used to build the UserInfo endpoint that validates
    # every request's Bearer token. Required: no default, so a deployment
    # that forgets to set it fails loudly at startup instead of silently
    # validating logins against the wrong tenant.
    ASGARDEO_ORG: str

    # Azure Blob Storage for evidence files. The connection string is required;
    # the backend has no local-filesystem fallback.
    AZURE_STORAGE_CONNECTION_STRING: str
    AZURE_STORAGE_CONTAINER: str = "uploads"

    # Comma-separated allow-list of emails granted the "admin" role. Everyone
    # else who authenticates gets "engineer". See app/auth.py.
    ADMIN_EMAILS: str = ""

    # Comma-separated CORS allow-list for the web frontend.
    CORS_ORIGINS: str = "http://localhost:5173,http://localhost:5174"

    # SQLAlchemy connection pool size, per process (this is a hard cap — see
    # app/database.py, which sets max_overflow=0). Each pod runs its own
    # process with its own pool, so DB_POOL_SIZE x max_pod_count must stay
    # comfortably under Postgres's own max_connections.
    DB_POOL_SIZE: int = 30


settings = Settings()
