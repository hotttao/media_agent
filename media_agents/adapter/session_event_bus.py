"""In-memory and RabbitMQ implementations of the Session Event Bus port."""

from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncGenerator

import aio_pika
from aio_pika.abc import (
    AbstractExchange,
    AbstractIncomingMessage,
    AbstractQueue,
    AbstractRobustChannel,
    AbstractRobustConnection,
)

from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification


class InMemorySessionEventBus:
    def __init__(self) -> None:
        self._subscribers: dict[str, set[asyncio.Queue[SessionEventNotification]]] = {}

    async def publish(self, notification: SessionEventNotification) -> None:
        for queue in tuple(self._subscribers.get(notification.session_id, set())):
            queue.put_nowait(notification)

    async def subscribe(self, session_id: str) -> AsyncGenerator[SessionEventNotification, None]:
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


class RabbitMQSessionEventBus:
    """One instance queue with local fan-out; publishing alone creates no queue."""

    def __init__(self, url: str, exchange: str = "media.agent.sessions.v1") -> None:
        self._url = url
        self._exchange_name = exchange
        self._connection: AbstractRobustConnection | None = None
        self._channel: AbstractRobustChannel | None = None
        self._exchange: AbstractExchange | None = None
        self._queue: AbstractQueue | None = None
        self._lock = asyncio.Lock()
        self._closed = False
        self._subscribers: dict[str, set[asyncio.Queue[SessionEventNotification | None]]] = {}

    @staticmethod
    def _routing_key(session_id: str) -> str:
        encoded = session_id.encode("utf-8")
        if not encoded or len(encoded) > 127:
            raise ValueError("invalid Session Event Bus session ID")
        return encoded.hex()

    async def _ensure_locked(self) -> AbstractExchange:
        if self._closed:
            raise RuntimeError("Session Event Bus closed")
        if self._exchange is None:
            try:
                connection = await aio_pika.connect_robust(
                    self._url, timeout=3, reconnect_interval=1, heartbeat=5
                )
                self._connection = connection
                channel = connection.channel(publisher_confirms=True)
                await channel.initialize()
                self._channel = channel
                await channel.set_qos(prefetch_count=256, timeout=3)
                self._exchange = await channel.declare_exchange(
                    self._exchange_name, aio_pika.ExchangeType.TOPIC, durable=True, timeout=3
                )
            except asyncio.CancelledError:
                if self._connection is not None:
                    await asyncio.shield(self._connection.close())
                    self._connection = None
                    self._channel = None
                raise
            except Exception:
                if self._connection is not None:
                    await self._connection.close()
                    self._connection = None
                    self._channel = None
                # URL credentials must not escape in transport exceptions.
                raise ConnectionError("connect Session Event Bus RabbitMQ failed") from None
        return self._exchange

    async def publish(self, notification: SessionEventNotification) -> None:
        key = self._routing_key(notification.session_id)
        if notification.sequence < 0:
            raise ValueError("negative Session Event sequence")
        payload = json.dumps(
            {
                "session_id": notification.session_id,
                "sequence": notification.sequence,
                "payload": notification.payload,
            },
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode("utf-8")
        async with self._lock:
            exchange = await self._ensure_locked()
        # No mandatory routing: Sessions may have no live viewers. The deadline
        # bounds reconnect waits; PostgreSQL remains the recovery source.
        await asyncio.wait_for(
            exchange.publish(
                aio_pika.Message(body=payload, content_type="application/json"),
                routing_key=key,
                mandatory=False,
                timeout=3,
            ),
            timeout=3,
        )

    async def subscribe(self, session_id: str) -> AsyncGenerator[SessionEventNotification, None]:
        key = self._routing_key(session_id)
        receiver: asyncio.Queue[SessionEventNotification | None] = asyncio.Queue(maxsize=256)
        async with self._lock:
            exchange = await self._ensure_locked()
            if self._queue is None:
                assert self._channel is not None
                queue = await self._channel.declare_queue(
                    "",
                    exclusive=True,
                    auto_delete=True,
                    arguments={
                        "x-max-length": 10000,
                        "x-message-ttl": 30000,
                        "x-queue-type": "classic",
                    },
                    timeout=3,
                )
                await queue.consume(self._on_message, timeout=3)
                self._queue = queue
            if session_id not in self._subscribers:
                await self._queue.bind(exchange, routing_key=key, timeout=3)
            self._subscribers.setdefault(session_id, set()).add(receiver)
        try:
            while (value := await receiver.get()) is not None:
                yield value
        finally:
            async with self._lock:
                subscribers = self._subscribers.get(session_id)
                if subscribers is not None:
                    subscribers.discard(receiver)
                    if not subscribers:
                        self._subscribers.pop(session_id, None)
                        if self._queue is not None and not self._closed:
                            try:
                                await asyncio.wait_for(
                                    self._queue.unbind(exchange, routing_key=key, timeout=3),
                                    timeout=3,
                                )
                            except (TimeoutError, ConnectionError, aio_pika.exceptions.AMQPError):
                                # A stale ephemeral binding only delivers ignorable hints.
                                pass

    async def _on_message(self, message: AbstractIncomingMessage) -> None:
        try:
            value = json.loads(message.body)
            if not isinstance(value, dict):
                return
            session_id, sequence, payload = (
                value.get("session_id"),
                value.get("sequence"),
                value.get("payload"),
            )
            if (
                not isinstance(session_id, str)
                or not session_id
                or type(sequence) is not int
                or sequence < 0
                or not isinstance(payload, dict)
            ):
                return
            notification = SessionEventNotification(session_id, sequence, payload)
            for receiver in tuple(self._subscribers.get(session_id, set())):
                if receiver.full():
                    while not receiver.empty():
                        receiver.get_nowait()
                    receiver.put_nowait(None)
                    self._subscribers[session_id].discard(receiver)
                else:
                    receiver.put_nowait(notification)
        except (ValueError, UnicodeDecodeError):
            pass
        finally:
            await message.ack()

    async def close(self) -> None:
        async with self._lock:
            self._closed = True
            for subscribers in self._subscribers.values():
                for receiver in subscribers:
                    while not receiver.empty():
                        receiver.get_nowait()
                    receiver.put_nowait(None)
            self._subscribers.clear()
            if self._connection is not None:
                await self._connection.close()
