"""Standard ACP fixture recording the actual per-Turn native request contract."""

from __future__ import annotations

import asyncio
import json
import os
import uuid
from pathlib import Path
from typing import Any, cast

from acp import Agent, run_agent
from acp.schema import AgentCapabilities, Implementation, InitializeResponse, NewSessionResponse


def record(value: dict[str, Any]) -> None:
    with Path(os.environ["MCP_CAPTURE"]).open("a") as output:
        output.write(json.dumps(value) + "\n")


class Fixture:
    async def initialize(self, protocol_version: int, **kwargs: object) -> InitializeResponse:
        return InitializeResponse(
            protocol_version=protocol_version,
            agent_capabilities=AgentCapabilities(load_session=True),
            agent_info=Implementation(name="mcp-fixture", version="1"),
        )

    def servers(self, operation: str, values: Any) -> None:
        record({"operation": operation, "servers": [v.model_dump(by_alias=True) for v in values]})

    async def new_session(self, mcp_servers: Any, **kwargs: object) -> NewSessionResponse:
        self.servers("new", mcp_servers)
        return NewSessionResponse(session_id=str(uuid.uuid4()))

    async def load_session(self, session_id: str, mcp_servers: Any, **kwargs: object) -> None:
        self.servers("load", mcp_servers)
        return None

    async def ext_method(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        assert method == "goose/unstable/session/system-prompt/set"
        record({"operation": "prompt", **params})
        return {}

    def on_connect(self, connection: object) -> None:
        pass


if __name__ == "__main__":
    asyncio.run(run_agent(cast(Agent, Fixture())))
