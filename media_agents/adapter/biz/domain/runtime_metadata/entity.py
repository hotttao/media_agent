from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Protocol


class InvalidMetadata(ValueError):
    def __init__(self, code: str, snapshot: Snapshot | None = None) -> None:
        super().__init__(code)
        self.snapshot = snapshot


@dataclass(frozen=True, slots=True)
class Resource:
    kind: str
    resource_id: str
    version: int
    name: str
    checksum: str
    content_ref: str = ""
    content: str = ""


@dataclass(frozen=True, slots=True)
class Snapshot:
    workspace_id: str
    agent_config_id: str
    selection_revision: int
    target_context_version: str
    context_version: int
    agent_type: str
    configured_prompt: str
    platform_prompt: str
    platform_prompt_template_version: str
    resources: tuple[Resource, ...]
    status: str = "current"


class MetadataGateway(Protocol):
    async def fetch(
        self, *, selection_revision: int = 0, target: str = "", for_session: bool = False
    ) -> Snapshot | None: ...

    async def artifact(self, target: str, checksum: str) -> bytes: ...

    async def report(self, result: dict[str, object]) -> None: ...


class VersionLoader(Protocol):
    async def resolve_session(self, context_ref: str) -> Path | None: ...
