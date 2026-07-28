"""use timestamptz for evidence and submission timestamps

Revision ID: 32f2e790ec7f
Revises: d510a724503d
Create Date: 2026-06-13 15:21:23.604113

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '32f2e790ec7f'
down_revision: Union[str, Sequence[str], None] = 'd510a724503d'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Existing naive timestamps were written under postgres TZ=UTC, so they
    # already represent UTC instants — reinterpret them as such.
    op.alter_column(
        "evidence", "created_at",
        type_=sa.DateTime(timezone=True),
        postgresql_using="created_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )
    op.alter_column(
        "evidence", "updated_at",
        type_=sa.DateTime(timezone=True),
        postgresql_using="updated_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )
    op.alter_column(
        "submissions", "submitted_at",
        type_=sa.DateTime(timezone=True),
        postgresql_using="submitted_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )


def downgrade() -> None:
    op.alter_column(
        "submissions", "submitted_at",
        type_=sa.DateTime(timezone=False),
        postgresql_using="submitted_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )
    op.alter_column(
        "evidence", "updated_at",
        type_=sa.DateTime(timezone=False),
        postgresql_using="updated_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )
    op.alter_column(
        "evidence", "created_at",
        type_=sa.DateTime(timezone=False),
        postgresql_using="created_at AT TIME ZONE 'UTC'",
        server_default=sa.func.now(),
    )
