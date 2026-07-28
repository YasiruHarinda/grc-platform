"""add products and link frameworks to products

Revision ID: 8b6ea913ff0e
Revises: 57b39d9adcfc
Create Date: 2026-06-09 13:54:21.888991

This migration:
  1. Wipes existing data from submissions, evidence, controls, frameworks
     (intentional - caller wants to start fresh and repopulate via the new UI)
  2. Drops the legacy uniqueness on frameworks.name (since same name can exist
     under different products now)
  3. Creates the products table
  4. Adds frameworks.product_id (NOT NULL, FK to products.id, ON DELETE CASCADE)
  5. Adds a composite uniqueness on (product_id, name)

Downgrade reverses the schema changes but does NOT restore deleted data.
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '8b6ea913ff0e'
down_revision: Union[str, Sequence[str], None] = '57b39d9adcfc'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # 1. Clear existing rows (children first to satisfy FKs)
    op.execute("DELETE FROM submissions")
    op.execute("DELETE FROM evidence")
    op.execute("DELETE FROM controls")
    op.execute("DELETE FROM frameworks")

    # 2. Drop the legacy unique constraint on frameworks.name
    op.drop_constraint("frameworks_name_key", "frameworks", type_="unique")

    # 3. Create products table
    op.create_table(
        "products",
        sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
        sa.Column("name", sa.String(length=100), nullable=False),
        sa.Column("description", sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("name"),
    )

    # 4. Add product_id to frameworks with cascade
    op.add_column(
        "frameworks",
        sa.Column("product_id", sa.Integer(), nullable=False),
    )
    op.create_foreign_key(
        "fk_frameworks_product_id",
        "frameworks",
        "products",
        ["product_id"],
        ["id"],
        ondelete="CASCADE",
    )

    # 5. Composite uniqueness (product_id, name)
    op.create_unique_constraint(
        "uq_framework_product_name",
        "frameworks",
        ["product_id", "name"],
    )


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_constraint("uq_framework_product_name", "frameworks", type_="unique")
    op.drop_constraint("fk_frameworks_product_id", "frameworks", type_="foreignkey")
    op.drop_column("frameworks", "product_id")
    op.drop_table("products")
    op.create_unique_constraint("frameworks_name_key", "frameworks", ["name"])
