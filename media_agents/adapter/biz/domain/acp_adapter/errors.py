"""ACP domain 错误。对标 internal/acp-adapter 在 /chat handler 里 surfacing 的错误。"""

from __future__ import annotations


class AcpError(Exception):
    """ACP domain 基类错误。"""


class BridgeNotFound(AcpError):
    """session 对应的 bridge 不存在（已驱逐或从未创建）。"""

    def __init__(self, session_id: str) -> None:
        super().__init__(f"No active session to reconnect: {session_id}")
        self.session_id = session_id


class LoadSessionFailed(AcpError):
    """loadSession 失败：session 不属于本 core / rollout 缺失。

    对标 acp-server.ts 的「Cannot continue this session ... most likely created
    under a different agent type」错误。
    """

    def __init__(self, session_id: str, detail: str) -> None:
        super().__init__(
            f"Cannot continue this session: its session record was not found. "
            f"It was most likely created under a different agent type, and sessions "
            f"cannot continue after switching agent type. Switch the workspace's agent "
            f"type back to the original one and try again. ({detail})"
        )
        self.session_id = session_id
        self.detail = detail
