"""add_kind_to_agent_tasks

Revision ID: f2a4c8e91b3d
Revises: d113b2c93af7
Create Date: 2026-06-16 10:05:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'f2a4c8e91b3d'
down_revision: Union[str, Sequence[str], None] = 'd113b2c93af7'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    op.add_column(
        'agent_tasks',
        sa.Column('kind', sa.String(length=20), nullable=False, server_default='run'),
    )


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_column('agent_tasks', 'kind')
