"""add control snapshot to agent_tasks

Revision ID: 16ebb55c4de1
Revises: fe1813bddbd8
Create Date: 2026-07-31 00:00:00.000000

`agent_tasks.control_id` is `ON DELETE SET NULL` (fe1813bddbd8): deleting a
Control detaches every Agent Task that targeted it, rather than blocking the
delete or destroying the task's audit history. That is the right rule for
Controls to stay deletable, but it has a side effect nobody intended --
`control_id` going NULL means three different things, and after the SET
NULL fires they are indistinguishable:

  1. A `login` task, which only opens a browser for manual MFA and never
     had a Control to begin with.
  2. A `run` task the user started without picking a Control.
  3. A run whose Control existed at the time, and was later deleted.

Nobody -- not the UI, not an Admin, not an auditor -- can tell these apart
once the FK has fired. "Show me every run against control AC-2" cannot be
answered after AC-2 is deleted, even though the task, its result and its
timings all still exist.

This migration adds two plain text columns with no foreign key of their
own, so the same `ON DELETE SET NULL` that clears `control_id` can never
reach them:

    control_ref_snapshot    VARCHAR(50)   -- e.g. "AC-2"
    control_title_snapshot  VARCHAR(255)  -- e.g. "Account Management"

They are populated once, at task creation (see `create_task` in
app/api/routes/agent.py), from whatever the Control was actually called at
that moment -- frozen, not kept in sync with later renames. That is
deliberate: the task should describe what it actually ran against, not
whatever the Control happens to be called today.

Both columns are added nullable, which is a lock-free, non-blocking DDL
change on Postgres and requires no default backfill for the column add
itself.

The separate UPDATE below is a *data* backfill, not a schema requirement:
it recovers the snapshot for every task whose Control still exists right
now, so the feature is useful immediately for existing history rather than
only for tasks created from this point on. It cannot help tasks whose
Control has *already* been deleted before this migration runs -- that
identity is already gone and there is nothing left to copy it from. That
gap is expected, not a bug in this migration, and is called out explicitly
in the PR rather than left to be discovered later.

This backfill is exercised only by running the migration for real -- the
test suite builds its schema straight from the models via `create_all` and
never invokes Alembic, so it must be checked by hand against a database
with real rows before this ships.
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '16ebb55c4de1'
down_revision: Union[str, Sequence[str], None] = 'fe1813bddbd8'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('agent_tasks', sa.Column('control_ref_snapshot', sa.String(length=50), nullable=True))
    op.add_column('agent_tasks', sa.Column('control_title_snapshot', sa.String(length=255), nullable=True))

    # Recover the identity for every task whose Control is still around --
    # see the module docstring for why this can't reach tasks whose Control
    # is already gone.
    op.execute(
        """
        UPDATE agent_tasks
           SET control_ref_snapshot   = c.control_ref,
               control_title_snapshot = c.title
          FROM controls c
         WHERE agent_tasks.control_id = c.id
        """
    )


def downgrade() -> None:
    op.drop_column('agent_tasks', 'control_title_snapshot')
    op.drop_column('agent_tasks', 'control_ref_snapshot')
