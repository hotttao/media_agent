"""ACP domain 实体 / 值对象。

对标 internal/types/events.ts 的 UniversalEvent / TurnStats / ContextGauge。
不依赖 FastAPI / Pydantic / SQLAlchemy，可被 domain/service/dal 共享。
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from adapter.biz.domain.acp.repository import AcpBridge


class StopReason(StrEnum):
    COMPLETED = "completed"
    CANCELLED = "cancelled"


@dataclass(frozen=True, slots=True)
class ContextGauge:
    """对标 ContextGauge：当前 context window 占用快照。"""

    num_turns: int
    context_tokens: int
    context_window: int


@dataclass(frozen=True, slots=True)
class TurnStats:
    """对标 TurnStats：单次 turn 的 token 计量。"""

    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0
    context: ContextGauge | None = None


@dataclass(slots=True)
class PromptResult:
    """对标 PromptResponse：bridge.prompt 的返回值。"""

    stop_reason: StopReason
    usage: Any | None = None  # agent-specific usage 对象


@dataclass(slots=True)
class AcpSession:
    """ACP 会话运行时状态。

    一个 session 绑定一个 AcpBridge 子进程；busy_turns 是并发 turn 引用计数，
    用于在 /reload-config 与 LRU 驱逐时避免销毁正在执行 turn 的 bridge。
    """

    session_id: str
    bridge: AcpBridge
    busy_turns: int = 0
    last_active: float = 0.0
    pending_destroy: bool = False
