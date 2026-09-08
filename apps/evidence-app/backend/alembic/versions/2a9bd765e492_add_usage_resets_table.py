"""add usage_resets table

Revision ID: 2a9bd765e492
Revises: 16ebb55c4de1
Create Date: 2026-09-08 00:00:00.000000

Adds the table backing the Cost & Usage page's Reset button (issue #126).
A Usage Reset is a recorded moment, not a mutation of a stored value: each
press of the button inserts a new row here, and the usage reports treat the
most recent `effective_at` as the cutoff for what they count. Existing
`usage_logs` rows are never touched by this or any later migration -- the
reports filter them, they do not delete them.
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '2a9bd765e492'
down_revision: Union[str, Sequence[str], None] = '16ebb55c4de1'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        'usage_resets',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('effective_at', sa.DateTime(timezone=True), nullable=False),
        sa.Column('reset_by', sa.String(length=255), nullable=False),
        sa.PrimaryKeyConstraint('id'),
    )
    op.create_index(op.f('ix_usage_resets_effective_at'), 'usage_resets', ['effective_at'], unique=False)


def downgrade() -> None:
    op.drop_index(op.f('ix_usage_resets_effective_at'), table_name='usage_resets')
    op.drop_table('usage_resets')
