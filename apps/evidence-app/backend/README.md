# Compliance Evidence Portal — Backend

FastAPI service that stores compliance evidence and coordinates the browser-automation
runner. Deployed on Choreo as a REST service (see `.choreo/component.yaml`).

## Stack

- **FastAPI** REST API (`app/main.py`)
- **PostgreSQL** (Neon in production) via SQLAlchemy + Alembic
- **Azure Blob Storage** for evidence files (required — there is no local fallback)
- **Asgardeo** OAuth2 for authentication (`app/auth.py`)

## Layout

```
app/
  main.py              FastAPI app, CORS, blob file serving, router registration
  config.py            Settings loaded from environment (.env)
  auth.py              Asgardeo bearer-token validation + dev header fallback
  rbac.py              require_admin / require_engineer_or_admin dependencies
  database.py          SQLAlchemy engine + session
  models/              ORM models
  schemas/             Pydantic request/response models
  api/routes/          One module per resource
  storage/             blob_storage.py — Azure Blob upload/delete
alembic/               Database migrations
```

## Local development

```bash
python3.11 -m venv venv
source venv/bin/activate
pip install -r requirements.txt

cp .env.example .env        # then fill in the values
alembic upgrade head
uvicorn app.main:app --reload --port 8000
```

Every request must carry a valid Asgardeo `Authorization: Bearer <token>` — the
same locally as in production. The web app obtains it via Asgardeo sign-in and
the Runner via its own Asgardeo login.

## Environment variables

See [`.env.example`](.env.example). All variables there are required except
`ADMIN_EMAILS` (may be empty) and the ones with defaults.

## Database migrations

```bash
alembic upgrade head                      # apply pending migrations
alembic revision --autogenerate -m "..."  # generate a migration from model changes
```

New models must be imported in `alembic/env.py` for autogenerate to detect them.

## Tests

Tests exercise the FastAPI app over HTTP (`TestClient`) with `get_current_user`
and `get_db` overridden — no real Asgardeo account needed, and no fake stands
in for Azure Blob Storage. `get_db` is backed by a throwaway **Postgres**
database (not SQLite, so Postgres-specific column types behave the same as in
production); each test runs in its own rolled-back transaction. Routes that
upload, sign or delete files talk to a real **Azurite** blob-storage emulator,
for the same reason: production behaviour, not an approximation of it. See
`tests/conftest.py` for the fixtures.

```bash
pip install -r requirements.txt -r requirements-test.txt

# start a throwaway test database (any Postgres works; this is one way)
docker run -d --name evidence-app-test-db \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres \
  -e POSTGRES_DB=evidence_app_test -p 5433:5432 postgres:16

# start a throwaway blob-storage emulator
docker run -d --name evidence-app-test-blob -p 10000:10000 \
  mcr.microsoft.com/azure-storage/azurite:latest \
  azurite-blob --blobHost 0.0.0.0 --skipApiVersionCheck

pytest
```

By default tests connect to
`postgresql://postgres:postgres@localhost:5433/evidence_app_test`; override
with the `TEST_DATABASE_URL` environment variable to point elsewhere — the
suite refuses to run its destructive setup (`CREATE TABLE` / `DROP TABLE`)
unless the database name is explicitly a test database (exactly `test`, or
ending in `_test`), so a mis-set URL can't destroy real data. Tables are
created from the SQLAlchemy models directly (not via Alembic) at the start of
the test session and dropped at the end.

The blob-storage emulator is expected at the host and port baked into
`AZURE_STORAGE_CONNECTION_STRING` in `tests/conftest.py` (`127.0.0.1:10000` by
default); the storage container is created once per run, and every blob the run
uploaded is removed at the end, so runs don't accumulate files.

**If you have `AZURE_STORAGE_CONNECTION_STRING` set in your environment, unset
it before running the tests.** `conftest.py` only supplies the emulator default
when the variable is absent, so an exported value wins — and these tests upload
blobs, delete blobs, and clear the container at teardown. The suite therefore
refuses to run unless the connection string names the emulator account
(`devstoreaccount1`), mirroring the database-name guard above: a mis-set value
can't touch real storage.
