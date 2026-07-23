
import asyncio

import json
from __future__ import annotations
from typing import Any, Protocol, runtime_checkable
from adapter.biz.domain.acp_adapter.bridge import AcpBridge
from adapter.biz.domain.acp_adapter.translator import AcpEventTranslator


@runtime_checkable
class AcpSessionHandler(Protocol):
    """对标 AcpSessionHandler：bridge 回调上层处理事件。"""


    def on_update(self, update: Any) -> None:
        """ACP session/update 通知翻译为 UniversalEvent 投递给 SSE sink。"""
        ...

    async def on_permission_request(self, req: Any) -> dict[str, Any]:
        """ACP permission/request 回调。默认接受第一个选项。"""
        ...


class SessionSink:
    """对标 SessionSink：会话事件 sink。"""
    def __init__(self, session_id: str):
        self.session_id = session_id
        self.stream = asyncio.Queue()

    async def write(self, event: str, data: Any) -> None:
        """写入事件到 sink。"""
        ...

    async def read(self):
        """读取事件从 sink。"""
        ...
        async for event in self.stream:
            yield event


class AcpSession(AcpSessionHandler):
    """对标 Session：一个 ACP 会话。"""
    def __init__(self, session_id: str, bridge: AcpBridge):
        self.session_id = session_id
        self.bridge = bridge
        self.sink = SessionSink(self.session_id)
        self.translator = AcpEventTranslator(session_id)

    async def read(self):
        """读取事件从 sink。"""
        ...
        return self.sink.read()

    async def keep_alive(self):
        """保持会话活跃。"""
        ...
        pass

    async def run_turn(self):
        """运行一个 turn。"""
        ...
        self.bridge.prompt()
    
    async def on_update(self, update: Any) -> None:
        """ACP session/update 通知翻译为 UniversalEvent 投递给 SSE sink。"""
        ...
        for event in self.translator.translate(update):
            self.sink.write('message', json.dumps(event))

    async def on_permission_request(self, req: Any) -> dict[str, Any]:
        """ACP permission/request 回调。默认接受第一个选项。"""
        ...