"""ACP agent API request DTOs (Pydantic v2).

对标 internal/acp-adapter/types.ts 的 ChatRequest。仅 biz/api/** 可 import。
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class ChatImageAttachment(BaseModel):
    """对标 ChatImageAttachment：base64 编码的图片附件。"""

    data: str = Field(..., description="base64 encoded image bytes")
    media_type: str = Field(..., description="image/png | image/jpeg | image/gif | image/webp")


class ChatRequest(BaseModel):
    """POST /chat 请求体。

    session_id 缺省表示新会话；session_token 是 CP 反向解析会话身份的不透明令牌，
    透传到 MCP 服务器头部（X-Session-Token），由 CP 代理翻译为 X-Tos-Session-Id。
    """

    message: str
    session_id: str | None = None
    images: list[ChatImageAttachment] | None = None
    session_token: str | None = None
    api_key: str | None = None


class ReloadConfigRequest(BaseModel):
    """POST /reload-config 请求体（可选 scope 选择性重载）。"""

    scope: list[str] | None = None  # ["config", "skills", "credentials"]
