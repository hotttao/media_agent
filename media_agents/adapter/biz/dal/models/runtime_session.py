"""Runtime Session relational row model."""

from datetime import datetime

from sqlalchemy import BigInteger, DateTime, ForeignKey, Index, String, UniqueConstraint, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from adapter.biz.dal.models.base import Base


class RuntimeSessionRow(Base):
    __tablename__ = "runtime_sessions"
    __table_args__ = {"schema": "media_agents"}

    session_id: Mapped[str] = mapped_column(String(64), primary_key=True)
    context_version: Mapped[int] = mapped_column(BigInteger, nullable=False)
    context_ref: Mapped[str] = mapped_column(String(512), nullable=False)
    status: Mapped[str] = mapped_column(String(16), nullable=False)
    chat_status: Mapped[str] = mapped_column(String(16), nullable=False)
    active_turn_id: Mapped[str | None] = mapped_column(String(64), nullable=True)
    runtime_session_id: Mapped[str | None] = mapped_column(String(128), nullable=True)
    next_turn_sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    next_event_sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)


class TurnRow(Base):
    __tablename__ = "turns"
    __table_args__ = (
        UniqueConstraint("session_id", "idempotency_key", name="uq_turns_session_idempotency"),
        UniqueConstraint("session_id", "enqueue_sequence", name="uq_turns_session_sequence"),
        Index(
            "uq_turns_one_running",
            "session_id",
            unique=True,
            postgresql_where=text("status = 'running'"),
        ),
        {"schema": "media_agents"},
    )

    turn_id: Mapped[str] = mapped_column("id", String(64), primary_key=True)
    session_id: Mapped[str] = mapped_column(
        ForeignKey("media_agents.runtime_sessions.session_id"), nullable=False
    )
    idempotency_key: Mapped[str] = mapped_column(String(128), nullable=False)
    enqueue_sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    status: Mapped[str] = mapped_column(String(16), nullable=False)
    input: Mapped[dict[str, object]] = mapped_column(JSONB, nullable=False)
    context_version: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    error_class: Mapped[str | None] = mapped_column(String(64))


class MessageRow(Base):
    __tablename__ = "messages"
    __table_args__ = (
        UniqueConstraint("session_id", "sequence", name="uq_messages_session_sequence"),
        {"schema": "media_agents"},
    )

    message_id: Mapped[str] = mapped_column("id", String(64), primary_key=True)
    session_id: Mapped[str] = mapped_column(
        ForeignKey("media_agents.runtime_sessions.session_id"), nullable=False
    )
    turn_id: Mapped[str] = mapped_column(ForeignKey("media_agents.turns.id"), nullable=False)
    role: Mapped[str] = mapped_column(String(16), nullable=False)
    content_ref: Mapped[dict[str, object]] = mapped_column(JSONB, nullable=False)
    sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
