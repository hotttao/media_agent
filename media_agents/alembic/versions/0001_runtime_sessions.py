"""create runtime_sessions

Revision ID: 0001_runtime_sessions
Revises:
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "0001_runtime_sessions"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.execute("CREATE SCHEMA IF NOT EXISTS media_agents")
    op.create_table(
        "runtime_sessions",
        sa.Column("session_id", sa.String(length=64), nullable=False),
        sa.Column("context_version", sa.BigInteger(), nullable=False),
        sa.Column("context_ref", sa.String(length=512), nullable=False),
        sa.Column("status", sa.String(length=16), nullable=False),
        sa.Column("chat_status", sa.String(length=16), nullable=False),
        sa.Column("active_turn_id", sa.String(length=64), nullable=True),
        sa.Column("next_turn_sequence", sa.BigInteger(), nullable=False),
        sa.Column("next_event_sequence", sa.BigInteger(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("context_version > 0", name="ck_runtime_sessions_context_version"),
        sa.CheckConstraint(
            "status IN ('active','paused','failed','deleted')",
            name="ck_runtime_sessions_status",
        ),
        sa.CheckConstraint(
            "chat_status IN ('human','agent')", name="ck_runtime_sessions_chat_status"
        ),
        sa.CheckConstraint("next_turn_sequence > 0", name="ck_runtime_sessions_next_turn_sequence"),
        sa.CheckConstraint(
            "next_event_sequence > 0", name="ck_runtime_sessions_next_event_sequence"
        ),
        sa.PrimaryKeyConstraint("session_id"),
        schema="media_agents",
    )


def downgrade() -> None:
    op.drop_table("runtime_sessions", schema="media_agents")
