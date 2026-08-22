"""create Agent-owned Universal Event history and Message counter

Revision ID: 0004_event_history
Revises: 0003_runtime_handle
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "0004_event_history"
down_revision: str | None = "0003_runtime_handle"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "runtime_sessions",
        sa.Column("next_message_sequence", sa.BigInteger(), nullable=True),
        schema="media_agents",
    )
    op.execute(
        """
        UPDATE media_agents.runtime_sessions AS session
        SET next_message_sequence = COALESCE((
            SELECT MAX(message.sequence) + 1
            FROM media_agents.messages AS message
            WHERE message.session_id = session.session_id
        ), 1)
        """
    )
    op.alter_column(
        "runtime_sessions", "next_message_sequence", nullable=False, schema="media_agents"
    )
    op.create_table(
        "universal_events",
        sa.Column("id", sa.String(length=64), nullable=False),
        sa.Column(
            "session_id",
            sa.String(length=64),
            sa.ForeignKey("media_agents.runtime_sessions.session_id"),
            nullable=False,
        ),
        sa.Column(
            "turn_id",
            sa.String(length=64),
            sa.ForeignKey("media_agents.turns.id"),
            nullable=True,
        ),
        sa.Column("sequence", sa.BigInteger(), nullable=False),
        sa.Column("type", sa.String(length=64), nullable=False),
        sa.Column("payload", postgresql.JSONB(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("session_id", "sequence", name="uq_universal_events_session_sequence"),
        schema="media_agents",
    )
    op.create_index(
        "ix_universal_events_session_sequence",
        "universal_events",
        ["session_id", "sequence"],
        schema="media_agents",
    )


def downgrade() -> None:
    op.drop_index(
        "ix_universal_events_session_sequence",
        table_name="universal_events",
        schema="media_agents",
    )
    op.drop_table("universal_events", schema="media_agents")
    op.drop_column("runtime_sessions", "next_message_sequence", schema="media_agents")
