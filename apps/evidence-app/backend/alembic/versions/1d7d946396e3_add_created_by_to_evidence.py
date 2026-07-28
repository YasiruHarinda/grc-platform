"""add created_by to evidence

Revision ID: 1d7d946396e3
Revises: 32f2e790ec7f
Create Date: 2026-06-15 10:56:33.633793

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '1d7d946396e3'
down_revision: Union[str, Sequence[str], None] = '32f2e790ec7f'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Add nullable first so existing rows survive, backfill, then enforce NOT NULL.
    op.add_column(
        "evidence",
        sa.Column("created_by", sa.String(length=255), nullable=True),
    )
    op.execute("UPDATE evidence SET created_by = 'legacy@wso2.com' WHERE created_by IS NULL")
    op.alter_column("evidence", "created_by", nullable=False)


def downgrade() -> None:
    op.drop_column("evidence", "created_by")
