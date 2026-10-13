"""Instance identity and admission are stable execution invariants."""

from dataclasses import dataclass
from typing import Protocol

from adapter.biz.domain.runtime_session.entity import RuntimeSession
from adapter.biz.domain.runtime_session.errors import InvalidRuntimeSession


@dataclass(frozen=True, slots=True)
class InstanceScope:
    workspace_id: str
    agent_config_id: str
    replica_id: str
    incarnation_id: str

    def __post_init__(self) -> None:
        if any(
            not value or len(value) > 128
            for value in (
                self.workspace_id,
                self.agent_config_id,
                self.replica_id,
                self.incarnation_id,
            )
        ):
            raise InvalidRuntimeSession("invalid_instance_scope")


@dataclass(frozen=True, slots=True)
class Admission:
    scope: InstanceScope
    state: str = "active"
    operation_id: str = ""
    version: int = 1


class OwnershipRepository(Protocol):
    async def admission(
        self, scope: InstanceScope, *, for_update: bool = False
    ) -> Admission | None: ...
    async def save_admission(self, value: Admission) -> None: ...
    async def initialize_admission(self, scope: InstanceScope) -> Admission: ...
    async def sessions(self, scope: InstanceScope) -> list[RuntimeSession]: ...
    async def fence(self, scope: InstanceScope, epoch: int, *, transfer: bool = False) -> None: ...

    async def resume_once(self, session_id: str, operation_id: str, epoch: int) -> bool: ...
