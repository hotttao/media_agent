"""Generic ACP bridge built on the Python ACP SDK.

Agent packages provide concrete process options, while this class owns the ACP
handshake and session/prompt lifecycle.
"""

from __future__ import annotations

import asyncio
import logging
import os
from typing import TYPE_CHECKING

from acp import PROTOCOL_VERSION, connect_to_agent, image_block, text_block
from acp.schema import (
    ClientCapabilities,
    HttpMcpServer,
    Implementation,
    LoadSessionResponse,
    McpServerStdio,
    NewSessionResponse,
    SseMcpServer,
)

from adapter.biz.domain.acp_adapter.client import AcpClientAdapter
from adapter.biz.domain.acp_adapter.entity import PromptResult, StopReason
from adapter.biz.domain.acp_adapter.errors import LoadSessionFailed
from adapter.biz.domain.acp_adapter.llm_event import ChatImageAttachment
from adapter.biz.domain.acp_adapter.session import AcpSession

if TYPE_CHECKING:
    from acp.client.connection import ClientSideConnection

logger = logging.getLogger(__name__)

McpServerConfig = HttpMcpServer | SseMcpServer | McpServerStdio


class AcpBridge:
    """One child process bridge for one ACP session."""

    def __init__(
        self,
        *,
        program: str,
        args: list[str] | None = None,
        cwd: str = "/workspace",
        env: dict[str, str] | None = None,
        session_id: str = "",
    ) -> None:
        self.program = program
        self.args = args or []
        self.cwd = cwd
        self.env = env or {}
        self.session_id = session_id
        self.proc: asyncio.subprocess.Process | None = None
        self.conn: ClientSideConnection | None = None
        self.session: AcpSession | None = None
        self.client: AcpClientAdapter | None = None

    async def start(self) -> None:
        """Start the child process and complete the ACP initialize handshake."""
        env = {**os.environ, **self.env}
        self.proc = await asyncio.create_subprocess_exec(
            self.program,
            *self.args,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.DEVNULL,
            cwd=self.cwd,
            env=env,
        )
        self.session = AcpSession(self.session_id, self)
        self.client = AcpClientAdapter(self.session)
        self.conn = connect_to_agent(
            client=self.client,
            input_stream=self.proc.stdin,
            output_stream=self.proc.stdout,
        )
        await self.conn.initialize(
            protocol_version=PROTOCOL_VERSION,
            client_capabilities=ClientCapabilities(),
            client_info=Implementation(
                name="acp-bridge",
                title="ACP Bridge",
                version="0.1.0",
            ),
        )
        logger.info("acp bridge started: program=%s pid=%s", self.program, self.proc.pid)

    def is_alive(self) -> bool:
        return self.proc is not None and self.proc.returncode is None

    async def create_session(
        self, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        assert self.conn is not None and self.session is not None, "bridge not started"
        resp: NewSessionResponse = await self.conn.new_session(
            cwd=self.cwd,
            mcp_servers=mcp_servers or [],
        )
        self.session.set_session_id(resp.session_id)
        self.session_id = resp.session_id
        return self.session

    async def load_session(
        self, session_id: str, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        assert self.conn is not None and self.session is not None, "bridge not started"
        try:
            resp: LoadSessionResponse | None = await self.conn.load_session(
                cwd=self.cwd,
                session_id=session_id,
                mcp_servers=mcp_servers or [],
            )
        except Exception as err:
            raise LoadSessionFailed(session_id, str(err)) from err
        if resp is not None and getattr(resp, "modes", None):
            logger.debug("acp session loaded: session=%s modes=%s", session_id, resp.modes)
        return self.session

    async def wait_for_mcp_ready(self) -> None:
        """Hook for agents that expose an MCP-ready extension."""

    async def prompt(
        self, session_id: str, message: str, images: list[ChatImageAttachment] | None = None
    ) -> PromptResult:
        assert self.conn is not None, "bridge not started"
        prompt: list[object] = [text_block(message)]
        if images:
            for img in images:
                data = getattr(img, "data", None) or (
                    img.get("data") if isinstance(img, dict) else ""
                )
                media_type = getattr(img, "media_type", None) or (
                    img.get("media_type") if isinstance(img, dict) else ""
                )
                if data and media_type:
                    prompt.append(image_block(data, media_type))

        resp = await self.conn.prompt(session_id=session_id, prompt=prompt)
        raw_stop = getattr(resp, "stop_reason", None)
        stop = StopReason(raw_stop) if raw_stop else StopReason.END_TURN
        return PromptResult(stop_reason=stop, usage=getattr(resp, "usage", None))

    def cancel(self, session_id: str) -> None:
        if self.conn:
            asyncio.create_task(self.conn.cancel(session_id))

    def destroy(self) -> None:
        if self.conn:
            asyncio.create_task(self.conn.close())
        if self.proc and self.proc.returncode is None:
            self.proc.terminate()
        if self.session:
            self.session.close()
