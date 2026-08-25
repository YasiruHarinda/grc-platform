import re
from logging.config import fileConfig

from sqlalchemy import engine_from_config
from sqlalchemy import pool
from sqlalchemy import text

from alembic import context

from app.database import Base
from app.models import product, framework, control, evidence, evidence_file, submission, usage_log, agent_task  # noqa: F401
from app.config import settings

config = context.config

if config.config_file_name is not None:
    fileConfig(config.config_file_name)

# set_main_option writes into a ConfigParser, which reads "%" as the start of an
# interpolation token. A password with a character that has to be percent-encoded
# in the URL — "?" becomes "%3F" — therefore crashes the parser before a single
# connection is attempted. Doubling every "%" is how that parser is told to treat
# one literally; it hands the original string back, so SQLAlchemy still receives
# the URL exactly as it was written. A URL with no "%" in it is left untouched.
config.set_main_option("sqlalchemy.url", settings.DATABASE_URL.replace("%", "%%"))

target_metadata = Base.metadata

# other values from the config, defined by the needs of env.py,
# can be acquired:
# my_important_option = config.get_main_option("my_important_option")
# ... etc.

# DB_SCHEMA is configuration, not a literal any caller controls at request
# time, but it still ends up interpolated straight into DDL below — schema
# and database names cannot be bind parameters, Postgres only allows them as
# literal identifiers in this position. A plain identifier is the only shape
# that is safe to interpolate, so anything else is refused here rather than
# quoted and hoped for.
_SCHEMA_NAME_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def _create_and_switch_to_schema(connection, schema: str) -> None:
    """Give the app a schema of its own to create, own and migrate into,
    instead of the database's default "public" schema.

    Postgres 15+ only lets a schema's *owner* create objects inside it, so on
    a server where the app user does not own "public" — Azure Database for
    PostgreSQL is one, it hands "public" to its own admin role — migrations
    fail with a permission error before a single table exists. Creating a
    schema here means the app user owns it outright, which needs no grant
    from anyone.

    Three statements, three different jobs:

    - CREATE SCHEMA IF NOT EXISTS makes the schema. The IF NOT EXISTS is what
      makes this safe to run on every pod restart, not just the first.
    - SET search_path fixes *this* connection, the one Alembic is about to
      run all the migrations on. ALTER DATABASE ... SET below does not reach
      a connection that is already open, and this one already is — skip
      this line and the tables still land in "public", or the CREATE TABLE
      fails with the original permission error, depending on what "public"
      allows.
    - ALTER DATABASE ... SET fixes every *future* connection to this
      database, which is what lets app/database.py go untouched: the app's
      own engine picks up the new default the same way a fresh psql session
      would.
    """
    if not _SCHEMA_NAME_RE.match(schema):
        raise ValueError(
            f"DB_SCHEMA={schema!r} is not a valid identifier. It is "
            "interpolated directly into DDL because Postgres has no way to "
            "bind a schema name as a query parameter, so only a plain "
            f"identifier matching {_SCHEMA_NAME_RE.pattern!r} is accepted."
        )

    preparer = connection.dialect.identifier_preparer
    quoted_schema = preparer.quote(schema)

    database = connection.execute(text("SELECT current_database()")).scalar()
    quoted_database = preparer.quote(database)

    connection.execute(text(f"CREATE SCHEMA IF NOT EXISTS {quoted_schema}"))
    connection.execute(text(f"SET search_path TO {quoted_schema}"))
    connection.execute(
        text(f"ALTER DATABASE {quoted_database} SET search_path TO {quoted_schema}")
    )

    # engine_from_config below is built with NullPool, and this connection's
    # own execute() calls each opened an implicit transaction (SQLAlchemy 2.0
    # default) — without an explicit commit here, that transaction would
    # still be open when context.begin_transaction() starts the migrations'
    # own transaction next, leaving the schema uncommitted for the duration.
    connection.commit()


def run_migrations_offline() -> None:
    """Run migrations in 'offline' mode.

    This configures the context with just a URL
    and not an Engine, though an Engine is acceptable
    here as well.  By skipping the Engine creation
    we don't even need a DBAPI to be available.

    Calls to context.execute() here emit the given string to the
    script output.

    """
    url = config.get_main_option("sqlalchemy.url")
    context.configure(
        url=url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
    )

    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    """Run migrations in 'online' mode.

    In this scenario we need to create an Engine
    and associate a connection with the context.

    """
    connectable = engine_from_config(
        config.get_section(config.config_ini_section, {}),
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )

    with connectable.connect() as connection:
        if settings.DB_SCHEMA:
            _create_and_switch_to_schema(connection, settings.DB_SCHEMA)

        context.configure(
            connection=connection, target_metadata=target_metadata
        )

        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
