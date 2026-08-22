from pydantic import BaseModel


class RuntimeSessionResponse(BaseModel):
    session_id: str
    context_version: int
    context_ref: str
    status: str
    chat_status: str
    active_turn_id: str | None
    next_turn_sequence: int
    next_event_sequence: int
    created_at: str
    updated_at: str
