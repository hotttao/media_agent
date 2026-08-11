"""Codex ACP bridge factory wrapper."""

from __future__ import annotations

from adapter.biz.domain.acp_adapter.bridge import AcpBridge

DEFAULT_CODEX_ACP_PROGRAM = "codex-acp"
DEFAULT_INITIAL_AGENT_MODE = "agent-full-access"


class CodexAcpBridge(AcpBridge):
    """Codex-specific AcpBridge configuration.

    The ACP protocol implementation lives in ``adapter.biz.domain.acp_adapter``.
    This class only supplies codex-acp defaults and the sandbox env required by
    codex-acp 1.1.x.
    """

    def __init__(
        self,
        *,
        program: str = DEFAULT_CODEX_ACP_PROGRAM,
        args: list[str] | None = None,
        cwd: str = "/workspace",
        initial_agent_mode: str = DEFAULT_INITIAL_AGENT_MODE,
        session_id: str = "",
        api_key: str | None = None,
    ) -> None:
        super().__init__(
            program=program,
            args=args,
            cwd=cwd,
            env={"INITIAL_AGENT_MODE": initial_agent_mode, **({"OPENAI_API_KEY": api_key} if api_key else {})},
            session_id=session_id,
        )
