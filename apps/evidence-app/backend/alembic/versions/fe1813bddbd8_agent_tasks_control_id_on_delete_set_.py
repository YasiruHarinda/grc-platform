"""agent_tasks control_id on delete set null

Revision ID: fe1813bddbd8
Revises: a1b2c3d4e5f6
Create Date: 2026-07-28 00:00:00.000000

`agent_tasks.control_id` was a foreign key to `controls.id` with no ON
DELETE rule (plain RESTRICT), and `agent_tasks` was the only child of the
Product -> Framework -> Control hierarchy with no SQLAlchemy
`relationship()`/cascade, so the ORM never clears it before Postgres sees
the delete. A Control (or anything above it) with any agent history --
even a completed task from months ago, nothing clears `control_id` once a
task finishes -- could never be deleted; Postgres refused the delete and
the route handlers turned that refusal into a 409.

`ON DELETE CASCADE` was rejected: it would destroy audit history and strand
`usage_logs` rows, which reference the task id as a plain string with no FK
of their own. `ON DELETE SET NULL` keeps the Agent Task row (status,
result, timestamps all intact) and just detaches it from the Control that
no longer exists -- `agent_tasks.control_id` is already nullable, and so is
`evidence.control_id`, which the same detachment already happens to today.

The FK was created unnamed in d113b2c93af7_add_agent_tasks_table.py
(`sa.ForeignKeyConstraint(['control_id'], ['controls.id'], )`), so Postgres
auto-generated its name. Altering a FK's ON DELETE rule means dropping and
recreating the constraint, and dropping requires its name -- looked up here
from `information_schema` rather than assumed, since an auto-generated name
is an implementation detail, not a guarantee.
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'fe1813bddbd8'
down_revision: Union[str, Sequence[str], None] = 'a1b2c3d4e5f6'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _find_control_id_fk_name(conn) -> str:
    """The real name Postgres assigned to the FK on agent_tasks.control_id
    referencing controls.id -- looked up rather than hardcoded, because the
    constraint was created unnamed (see d113b2c93af7) and the conventional
    `agent_tasks_control_id_fkey` is only ever a likely guess, not a fact."""
    row = conn.execute(sa.text(
        """
        SELECT tc.constraint_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage kcu
          ON kcu.constraint_name = tc.constraint_name
         AND kcu.constraint_schema = tc.constraint_schema
        JOIN information_schema.constraint_column_usage ccu
          ON ccu.constraint_name = tc.constraint_name
         AND ccu.constraint_schema = tc.constraint_schema
        WHERE tc.constraint_type = 'FOREIGN KEY'
          -- current_schema() rather than a hardcoded 'public': it is
          -- whatever schema an unqualified table name resolves to right
          -- now, which is exactly what this lookup needs, and it is
          -- 'public' unless DB_SCHEMA (see app/config.py) says otherwise.
          AND tc.table_schema = current_schema()
          AND tc.table_name = 'agent_tasks'
          AND kcu.column_name = 'control_id'
          AND ccu.table_name = 'controls'
        """
    )).fetchone()
    if row is None:
        raise RuntimeError(
            "Could not find the foreign key constraint from agent_tasks.control_id "
            "to controls.id in information_schema; expected the one created by "
            "d113b2c93af7_add_agent_tasks_table.py."
        )
    return row[0]


def upgrade() -> None:
    conn = op.get_bind()
    fk_name = _find_control_id_fk_name(conn)
    op.drop_constraint(fk_name, 'agent_tasks', type_='foreignkey')
    op.create_foreign_key(
        'agent_tasks_control_id_fkey',
        'agent_tasks', 'controls',
        ['control_id'], ['id'],
        ondelete='SET NULL',
    )


def downgrade() -> None:
    conn = op.get_bind()
    fk_name = _find_control_id_fk_name(conn)
    op.drop_constraint(fk_name, 'agent_tasks', type_='foreignkey')
    op.create_foreign_key(
        'agent_tasks_control_id_fkey',
        'agent_tasks', 'controls',
        ['control_id'], ['id'],
    )
