"""Commands accepted by Runtime Session use cases."""

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class RegisterRuntimeSession:
    session_id: str
    context_version: int
    context_ref: str
    workspace_id: str = ""
    agent_config_id: str = ""
    initial_epoch: int = 1
    transfer_id: str = ""


@dataclass(frozen=True, slots=True)
class SubmitTurn:
    session_id: str
    idempotency_key: str
    context_version: int
    input: dict[str, object]
