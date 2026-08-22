"""In-memory and Redis implementations of the Session Event Bus port."""

from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator

from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification


class InMemorySessionEventBus:
    def __init__(self) -> None:
        self._subscribers: dict[str, set[asyncio.Queue[SessionEventNotification]]] = {}

    async def publish(self, notification: SessionEventNotification) -> None:
        for queue in tuple(self._subscribers.get(notification.session_id, set())):
            queue.put_nowait(notification)

    async def subscribe(self, session_id: str) -> AsyncIterator[SessionEventNotification]:
        queue: asyncio.Queue[SessionEventNotification] = asyncio.Queue()
        subscribers = self._subscribers.setdefault(session_id, set())
        subscribers.add(queue)
        try:
            while True:
                yield await queue.get()
        finally:
            subscribers.discard(queue)
            if not subscribers:
                self._subscribers.pop(session_id, None)

    async def close(self) -> None:
        self._subscribers.clear()


class RedisSessionEventBus:
    def __init__(self, url: str, channel_prefix: str = "media-agents:session-events") -> None:
        from redis.asyncio import Redis

        self._redis = Redis.from_url(url, decode_responses=True)
        self._prefix = channel_prefix

    async def publish(self, notification: SessionEventNotification) -> None:
        payload = json.dumps(
            {
                "session_id": notification.session_id,
                "sequence": notification.sequence,
                "payload": notification.payload,
            },
            ensure_ascii=False,
            separators=(",", ":"),
        )
        await self._redis.publish(self._channel(notification.session_id), payload)

    async def subscribe(self, session_id: str) -> AsyncIterator[SessionEventNotification]:
        pubsub = self._redis.pubsub()
        await pubsub.subscribe(self._channel(session_id))
        try:
            async for message in pubsub.listen():
                if message.get("type") != "message":
                    continue
                raw = message.get("data")
                value = json.loads(raw) if isinstance(raw, str) else {}
                payload = value.get("payload")
                if not isinstance(payload, dict):
                    continue
                yield SessionEventNotification(
                    session_id=str(value.get("session_id") or session_id),
                    sequence=int(value.get("sequence") or 0),
                    payload=payload,
                )
        finally:
            await pubsub.unsubscribe(self._channel(session_id))
            await pubsub.aclose()  # type: ignore[no-untyped-call]

    async def close(self) -> None:
        await self._redis.aclose()

    def _channel(self, session_id: str) -> str:
        return f"{self._prefix}:{session_id}"
