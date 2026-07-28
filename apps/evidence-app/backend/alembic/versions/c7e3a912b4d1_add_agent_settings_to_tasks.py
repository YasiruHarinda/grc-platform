"""add_agent_settings_to_tasks

Revision ID: c7e3a912b4d1
Revises: f2a4c8e91b3d
Create Date: 2026-06-10 10:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = 'c7e3a912b4d1'
down_revision: Union[str, Sequence[str], None] = 'f2a4c8e91b3d'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('agent_tasks', sa.Column('max_steps', sa.Integer(), nullable=True))
    op.add_column('agent_tasks', sa.Column('use_vision', sa.Boolean(), nullable=True))
    op.add_column('agent_tasks', sa.Column('max_actions_per_step', sa.Integer(), nullable=True))


def downgrade() -> None:
    op.drop_column('agent_tasks', 'max_actions_per_step')
    op.drop_column('agent_tasks', 'use_vision')
    op.drop_column('agent_tasks', 'max_steps')
