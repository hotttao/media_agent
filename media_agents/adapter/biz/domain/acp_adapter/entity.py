"""ACP domain 实体 / 值对象。

对标 internal/types/events.ts 的 UniversalEvent / TurnStats / ContextGauge。
不依赖 FastAPI / Pydantic / SQLAlchemy，可被 domain/service/dal 共享。
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from adapter.biz.domain.acp_adapter.bridge import AcpBridge


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


from pydantic import BaseModel, Field


class ChatImageAttachment(BaseModel):
    """对标 ChatImageAttachment：base64 编码的图片附件。"""

    data: str = Field(..., description="base64 encoded image bytes")
    media_type: str = Field(..., description="image/png | image/jpeg | image/gif | image/webp")


class Chat(BaseModel):
    """POST /chat 请求体。

    session_id 缺省表示新会话；session_token 是 CP 反向解析会话身份的不透明令牌，
    透传到 MCP 服务器头部（X-Session-Token），由 CP 代理翻译为 X-Tos-Session-Id。
    """

    message: str
    session_id: str | None = None
    images: list[ChatImageAttachment] | None = None
    session_token: str | None = None