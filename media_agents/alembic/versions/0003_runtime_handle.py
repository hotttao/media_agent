"""add durable Runtime handle

Revision ID: 0003_runtime_handle
Revises: 0002_turn_fifo
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "0003_runtime_handle"
down_revision: str | None = "0002_turn_fifo"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "runtime_sessions",
        sa.Column("runtime_session_id", sa.String(length=128), nullable=True),
        schema="media_agents",
    )


def downgrade() -> None:
    op.drop_column("runtime_sessions", "runtime_session_id", schema="media_agents")
