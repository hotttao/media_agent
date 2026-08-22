"""Runtime Session relational row model."""

from datetime import datetime

from sqlalchemy import BigInteger, DateTime, String
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
    next_turn_sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    next_event_sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
