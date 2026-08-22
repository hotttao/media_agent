from datetime import UTC, datetime

import pytest

from adapter.biz.domain.runtime_session.entity import ChatStatus, RuntimeSession, SessionStatus
from adapter.biz.domain.runtime_session.errors import InvalidRuntimeSession


def test_register_creates_active_human_controlled_session() -> None:
    now = datetime(2026, 8, 22, 10, 30, tzinfo=UTC)

    session = RuntimeSession.register(
        session_id=" session-1 ", context_version=3, context_ref=" context:v3 ", now=now
    )

    assert session.session_id == "session-1"
    assert session.context_version == 3
    assert session.context_ref == "context:v3"
    assert session.status is SessionStatus.ACTIVE
    assert session.chat_status is ChatStatus.HUMAN
    assert session.active_turn_id is None
    assert session.next_turn_sequence == 1
    assert session.next_event_sequence == 1
    assert session.created_at == now


@pytest.mark.parametrize(
    ("session_id", "context_version", "context_ref", "message"),
    [
        ("", 1, "ctx", "session_id is required"),
        ("session-1", 0, "ctx", "context_version must be positive"),
        ("session-1", 1, " ", "context_ref is required"),
    ],
)
def test_register_rejects_invalid_identity_and_context(
    session_id: str, context_version: int, context_ref: str, message: str
) -> None:
    with pytest.raises(InvalidRuntimeSession, match=message):
        RuntimeSession.register(
            session_id=session_id,
            context_version=context_version,
            context_ref=context_ref,
        )
