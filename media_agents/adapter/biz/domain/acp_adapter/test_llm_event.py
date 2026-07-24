from adapter.biz.domain.acp_adapter.llm_event import (
    ContentDelta,
    UniversalEvent,
    to_event_dict,
)


def test_llm_event_module_owns_universal_event_serialization() -> None:
    event = UniversalEvent(
        type="item.delta",
        timestamp=1,
        session_id="s1",
        item_id="i1",
        delta=ContentDelta(type="text", text="hello"),
    )

    assert to_event_dict(event) == {
        "type": "item.delta",
        "timestamp": 1,
        "session_id": "s1",
        "item_id": "i1",
        "delta": {"type": "text", "text": "hello"},
    }
