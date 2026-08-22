#!/usr/bin/env python3
"""Deterministic ACP agent used by media-cli permission E2E only."""

from __future__ import annotations

import asyncio
import uuid
from typing import cast

from acp import Agent, run_agent
from acp.core import AgentSideConnection
from acp.schema import (
    AgentCapabilities,
    AgentMessageChunk,
    Implementation,
    InitializeResponse,
    NewSessionResponse,
    PermissionOption,
    PromptResponse,
    TextContentBlock,
    ToolCallUpdate,
)


class FakeAgent:
    def __init__(self) -> None:
        self.connection: AgentSideConnection | None = None

    def on_connect(self, connection: AgentSideConnection) -> None:
        self.connection = connection

    async def initialize(self, protocol_version: int, **_kwargs: object) -> InitializeResponse:
        return InitializeResponse(
            protocol_version=protocol_version,
            agent_capabilities=AgentCapabilities(load_session=True),
            agent_info=Implementation(name="fake-acp", version="1.0.0"),
        )

    async def new_session(self, **_kwargs: object) -> NewSessionResponse:
        return NewSessionResponse(session_id=str(uuid.uuid4()))

    async def load_session(self, session_id: str, **_kwargs: object) -> None:
        return None

    async def prompt(self, session_id: str, **_kwargs: object) -> PromptResponse:
        assert self.connection is not None
        await self.connection.session_update(
            session_id,
            AgentMessageChunk(
                content=TextContentBlock(type="text", text="Permission required."),
                message_id=str(uuid.uuid4()),
                session_update="agent_message_chunk",
            ),
        )
        await self.connection.request_permission(
            session_id,
            ToolCallUpdate(
                tool_call_id="fake-permission-call",
                kind="execute",
                title="Run the permission E2E command",
                status="pending",
            ),
            [
                PermissionOption(
                    option_id="allow-once",
                    name="Allow once",
                    kind="allow_once",
                ),
                PermissionOption(
                    option_id="reject-once",
                    name="Reject",
                    kind="reject_once",
                ),
            ],
        )
        await self.connection.session_update(
            session_id,
            AgentMessageChunk(
                content=TextContentBlock(type="text", text="Permission resolved."),
                message_id=str(uuid.uuid4()),
                session_update="agent_message_chunk",
            ),
        )
        return PromptResponse(stop_reason="end_turn")


async def main() -> None:
    await run_agent(cast(Agent, FakeAgent()))


if __name__ == "__main__":
    asyncio.run(main())
