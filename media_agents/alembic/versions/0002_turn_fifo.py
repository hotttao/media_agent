"""create persistent Turn FIFO and user messages

Revision ID: 0002_turn_fifo
Revises: 0001_runtime_sessions
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "0002_turn_fifo"
down_revision: str | None = "0001_runtime_sessions"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "turns",
        sa.Column("id", sa.String(64), primary_key=True),
        sa.Column(
            "session_id",
            sa.String(64),
            sa.ForeignKey("media_agents.runtime_sessions.session_id"),
            nullable=False,
        ),
        sa.Column("idempotency_key", sa.String(128), nullable=False),
        sa.Column("enqueue_sequence", sa.BigInteger, nullable=False),
        sa.Column("status", sa.String(16), nullable=False),
        sa.Column("input", postgresql.JSONB, nullable=False),
        sa.Column("context_version", sa.BigInteger, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True)),
        sa.Column("finished_at", sa.DateTime(timezone=True)),
        sa.Column("error_class", sa.String(64)),
        sa.CheckConstraint(
            "status IN ('queued','running','completed','failed','interrupted','cancelled')",
            name="ck_turns_status",
        ),
        sa.UniqueConstraint("session_id", "idempotency_key", name="uq_turns_session_idempotency"),
        sa.UniqueConstraint("session_id", "enqueue_sequence", name="uq_turns_session_sequence"),
        schema="media_agents",
    )
    op.create_index(
        "uq_turns_one_running",
        "turns",
        ["session_id"],
        unique=True,
        schema="media_agents",
        postgresql_where=sa.text("status = 'running'"),
    )
    op.create_table(
        "messages",
        sa.Column("id", sa.String(64), primary_key=True),
        sa.Column(
            "session_id",
            sa.String(64),
            sa.ForeignKey("media_agents.runtime_sessions.session_id"),
            nullable=False,
        ),
        sa.Column("turn_id", sa.String(64), sa.ForeignKey("media_agents.turns.id"), nullable=False),
        sa.Column("role", sa.String(16), nullable=False),
        sa.Column("content_ref", postgresql.JSONB, nullable=False),
        sa.Column("sequence", sa.BigInteger, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("role IN ('user','assistant','tool','system')", name="ck_messages_role"),
        sa.UniqueConstraint("session_id", "sequence", name="uq_messages_session_sequence"),
        schema="media_agents",
    )


def downgrade() -> None:
    op.drop_table("messages", schema="media_agents")
    op.drop_index("uq_turns_one_running", table_name="turns", schema="media_agents")
    op.drop_table("turns", schema="media_agents")
