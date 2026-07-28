"""make evidence control_id nullable

Revision ID: e9f3a1b8c2d7
Revises: f71a8f5f6c90
Create Date: 2026-06-19 13:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = 'e9f3a1b8c2d7'
down_revision: Union[str, Sequence[str], None] = 'f71a8f5f6c90'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.alter_column('evidence', 'control_id',
                    existing_type=sa.Integer(),
                    nullable=True)


def downgrade() -> None:
    op.alter_column('evidence', 'control_id',
                    existing_type=sa.Integer(),
                    nullable=False)
