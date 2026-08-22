from pydantic import BaseModel


class RuntimeSessionResponse(BaseModel):
    session_id: str
    context_version: int
    context_ref: str
    status: str
    chat_status: str
    active_turn_id: str | None
    runtime_session_id: str | None
    next_turn_sequence: int
    next_event_sequence: int
    next_message_sequence: int
    created_at: str
    updated_at: str


class TurnResponse(BaseModel):
    turn_id: str
    session_id: str
    idempotency_key: str
    enqueue_sequence: int
    status: str
    context_version: int
    input: dict[str, object]
    created_at: str
    started_at: str | None
    finished_at: str | None
    error_class: str | None


class UniversalEventResponse(BaseModel):
    event_id: str
    session_id: str
    turn_id: str | None
    sequence: int
    event_type: str
    payload: dict[str, object]
    created_at: str


class MessageResponse(BaseModel):
    message_id: str
    session_id: str
    turn_id: str
    role: str
    content_ref: dict[str, object]
    sequence: int
    created_at: str


class AgentRequestResponse(BaseModel):
    request_id: str
    session_id: str
    turn_id: str
    request_type: str
    payload: dict[str, object]
    status: str
    response: dict[str, object] | None
    resolved_at: str | None
    created_at: str
