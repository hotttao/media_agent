"""ID 生成器：draft session UUID。"""

from __future__ import annotations

import uuid


def new_session_id() -> str:
    """对标 acp-server.ts /chat handler 里 randomUUID() 预制 draft session id。"""
    return str(uuid.uuid4())
