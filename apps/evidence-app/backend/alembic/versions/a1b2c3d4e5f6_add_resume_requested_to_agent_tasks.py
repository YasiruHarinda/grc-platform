"""add_resume_requested_to_agent_tasks

Revision ID: a1b2c3d4e5f6
Revises: 5f54afda0503
Create Date: 2026-07-13 15:30:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'a1b2c3d4e5f6'
down_revision: Union[str, Sequence[str], None] = '5f54afda0503'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    op.add_column(
        'agent_tasks',
        sa.Column('resume_requested', sa.Boolean(), nullable=False, server_default='0'),
    )


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_column('agent_tasks', 'resume_requested')
