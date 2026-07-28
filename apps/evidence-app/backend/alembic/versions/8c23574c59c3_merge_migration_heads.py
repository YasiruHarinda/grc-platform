"""merge migration heads

Revision ID: 8c23574c59c3
Revises: 52470bb3ef1e, c7e3a912b4d1
Create Date: 2026-06-18 09:51:33.741548

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '8c23574c59c3'
down_revision: Union[str, Sequence[str], None] = ('52470bb3ef1e', 'c7e3a912b4d1')
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    pass


def downgrade() -> None:
    """Downgrade schema."""
    pass
