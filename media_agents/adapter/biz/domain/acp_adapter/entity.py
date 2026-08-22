"""ACP domain 实体 / 值对象。

LLM event stream 协议值对象定义在 llm_event.py；此处只保留 ACP bridge
返回值与请求体。
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum

from adapter.biz.domain.acp_adapter.llm_event import ChatImageAttachment


# ── PromptResponse.stop_reason ────────────────────────────
# 对齐 SDK schema.StopReason = Literal["end_turn", "max_tokens",
# "max_turn_requests", "refusal", "cancelled"]（acp/schema.py:14）。用
# StrEnum 固化取值，便于比较与日志输出；bridge.prompt 返回时统一映射。
class StopReason(StrEnum):
    """PromptResponse.stop_reason 的取值（对齐 SDK schema.StopReason）。"""

    END_TURN = "end_turn"
    MAX_TOKENS = "max_tokens"
    MAX_TURN_REQUESTS = "max_turn_requests"
    REFUSAL = "refusal"
    CANCELLED = "cancelled"


# ── bridge.prompt 返回值 ──────────────────────────────────


@dataclass(slots=True)
class PromptResult:
    """bridge.prompt 的返回值。

    stop_reason 对齐 SDK schema.StopReason；usage 是 SDK schema.Usage 原样
    透传（agent-specific 字段在 _meta 内，由 agent 包自行解释）。
    """

    stop_reason: StopReason
    usage: object | None = None


# ── HTTP 请求体 ────────────────────────────────────────────


@dataclass(frozen=True, slots=True)
class Chat:
    """POST /chat 请求体。

    session_id 缺省表示新会话；session_token 是 CP 反向解析会话身份的不透明
    令牌，透传到 MCP 服务器头部（X-Session-Token），由 CP 代理翻译为
    X-Tos-Session-Id。
    """

    message: str
    session_id: str | None = None
    images: list[ChatImageAttachment] | None = None
    session_token: str | None = None
    api_key: str | None = None
