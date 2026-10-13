"""Goose uses standard ACP sessions and its documented system-prompt extension."""

from __future__ import annotations

import json
from collections.abc import Callable, Mapping
from pathlib import Path

from adapter.biz.domain.acp_adapter.bridge import AcpBridge, McpServerConfig
from adapter.biz.domain.acp_adapter.session import AcpSession


class GooseAcpBridge(AcpBridge):
    def __init__(
        self,
        *,
        program: str = "goose",
        args: list[str] | None = None,
        cwd: str = "/workspace",
        goose_path_root: str | None = None,
        session_id: str = "",
        api_key: str | None = None,
        on_ext_notification: Callable[[str, str, Mapping[str, object]], None] | None = None,
    ) -> None:
        super().__init__(
            program=program,
            args=args or ["acp", "--with-builtin", "developer"],
            cwd=cwd,
            session_id=session_id,
            env={"OPENAI_API_KEY": api_key} if api_key else {},
        )
        self.path_root = Path(goose_path_root or (Path(cwd) / ".metadata/sessions"))
        self.bundle: Path | None = None
        self.platform_session_id = session_id
        self.on_ext = on_ext_notification

    def configure_metadata(self, bundle: Path, session_id: str) -> None:
        self.bundle = bundle
        self.platform_session_id = session_id

    async def start(self) -> None:
        target = self.path_root / self.platform_session_id / "goose"
        target.mkdir(parents=True, exist_ok=True, mode=0o700)
        config = target / "config.yaml"
        # Session MCP declarations are supplied through ACP. Ambient extensions cannot
        # add backend servers from the process-global Goose configuration.
        config.write_text(json.dumps({"extensions": {}}))
        config.chmod(0o600)
        self.env["GOOSE_PATH_ROOT"] = str(target)
        await super().start()
        if self.client is not None:
            self.client.extension_handler = self._extension

    def _extension(self, method: str, params: dict[str, object]) -> None:
        if self.on_ext is not None:
            self.on_ext(self.platform_session_id, "_" + method, params)

    async def _instructions(self) -> None:
        if self.bundle is None:
            return
        assert self.conn is not None
        for key, filename in (
            ("platform", "platform_prompt.md"),
            ("configured", "configured_prompt.md"),
        ):
            await self.conn.ext_method(
                "goose/unstable/session/system-prompt/set",
                {
                    "sessionId": self.session_id,
                    "mode": "append",
                    "key": "media-agent-" + key,
                    "text": (self.bundle / filename).read_text(),
                },
            )

    async def create_session(
        self, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        session = await super().create_session(mcp_servers=mcp_servers)
        await self._instructions()
        return session

    async def load_session(
        self, session_id: str, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        session = await super().load_session(session_id, mcp_servers=mcp_servers)
        await self._instructions()
        return session
