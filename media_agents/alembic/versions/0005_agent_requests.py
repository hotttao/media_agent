"""persist Agent Requests and responses

Revision ID: 0005_agent_requests
Revises: 0004_event_history
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "0005_agent_requests"
down_revision: str | None = "0004_event_history"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "agent_requests",
        sa.Column("id", sa.String(64), primary_key=True),
        sa.Column(
            "session_id",
            sa.String(64),
            sa.ForeignKey("media_agents.runtime_sessions.session_id"),
            nullable=False,
        ),
        sa.Column("turn_id", sa.String(64), sa.ForeignKey("media_agents.turns.id"), nullable=False),
        sa.Column("type", sa.String(16), nullable=False),
        sa.Column("payload", postgresql.JSONB, nullable=False),
        sa.Column("status", sa.String(16), nullable=False),
        sa.Column("response", postgresql.JSONB),
        sa.Column("resolved_at", sa.DateTime(timezone=True)),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "type IN ('confirm','input','permission')", name="ck_agent_requests_type"
        ),
        sa.CheckConstraint(
            "status IN ('pending','resolved','cancelled','expired')",
            name="ck_agent_requests_status",
        ),
        schema="media_agents",
    )
    op.create_index(
        "ix_agent_requests_session_status",
        "agent_requests",
        ["session_id", "status"],
        schema="media_agents",
    )


def downgrade() -> None:
    op.drop_index(
        "ix_agent_requests_session_status", table_name="agent_requests", schema="media_agents"
    )
    op.drop_table("agent_requests", schema="media_agents")
