"""Compatibility wrapper for the ACP domain server.

The ACP orchestration now lives in `adapter.biz.domain.acp_adapter.AcpServer`.
This module keeps the old `SessionManager` import path working for callers that
have not moved yet.
"""

from __future__ import annotations

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.biz.domain.acp_adapter.entity import PromptResult as DomainPromptResult
from adapter.biz.domain.acp_adapter.session import AcpSession
from adapter.biz.shared.clock import Clock


class SessionManager(AcpServer):
    def __init__(
        self,
        *,
        bridge_factory: BridgeFactory,
        hooks: AgentHooks,
        idle_ttl_ms: int = 10 * 60 * 1000,
        max_count: int = 10,
        clock: Clock | None = None,
    ) -> None:
        super().__init__(
            agent_hooks=hooks,
            bridge_factory=bridge_factory,
            idle_ttl_ms=idle_ttl_ms,
            max_count=max_count,
            clock=clock,
        )


__all__: list[str] = ["SessionManager", "AcpServer", "AgentHooks", "BridgeFactory"]

# Preserve the old broad import surface for typing-only consumers.
AcpSessionHandler = AcpSession
PromptResult = DomainPromptResult
