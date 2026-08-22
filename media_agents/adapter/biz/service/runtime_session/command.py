"""Commands accepted by Runtime Session use cases."""

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class RegisterRuntimeSession:
    session_id: str
    context_version: int
    context_ref: str


@dataclass(frozen=True, slots=True)
class SubmitTurn:
    session_id: str
    idempotency_key: str
    context_version: int
    input: dict[str, object]
