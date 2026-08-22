"""Classify Runtime output, persist recoverable facts, and fan out notifications."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Awaitable, Callable
from typing import Protocol

from adapter.biz.domain.runtime_session.entity import UniversalEvent
from adapter.biz.domain.runtime_session.event_bus import (
    SessionEventBus,
    SessionEventNotification,
)
from adapter.biz.domain.runtime_session.runtime_event import (
    RuntimeEventEnvelope,
    normalize_tool_result,
)

logger = logging.getLogger(__name__)

RuntimeEventHandler = Callable[[dict[str, object]], Awaitable[None]]


class RuntimeEventStore(Protocol):
    async def persist_runtime_event(
        self, session_id: str, turn_id: str, payload: dict[str, object]
    ) -> UniversalEvent: ...


class TurnEventSink:
    """One-Turn event buffer; terminal flush is explicit and deterministic."""

    def __init__(
        self,
        service: RuntimeEventStore,
        bus: SessionEventBus,
        session_id: str,
        turn_id: str,
        *,
        tool_result_max_bytes: int = 32 * 1024,
        tool_result_window_ms: int = 300,
    ) -> None:
        self._service = service
        self._bus = bus
        self._session_id = session_id
        self._turn_id = turn_id
        self._max_bytes = tool_result_max_bytes
        self._window_seconds = tool_result_window_ms / 1000
        self._seen_tool_results: set[str] = set()
        self._pending: dict[str, dict[str, object]] = {}
        self._timers: dict[str, asyncio.Task[None]] = {}
        self._lock = asyncio.Lock()
        self._terminal_payload: dict[str, object] | None = None

    @property
    def terminal_payload(self) -> dict[str, object] | None:
        return self._terminal_payload

    async def publish_committed(self, event: UniversalEvent) -> None:
        await publish_event(self._bus, event)

    async def accept(self, payload: dict[str, object]) -> None:
        envelope = RuntimeEventEnvelope.parse(payload)
        if envelope.is_live:
            await publish_notification(
                self._bus,
                SessionEventNotification(self._session_id, 0, payload),
            )
            return
        if envelope.is_terminal:
            self._terminal_payload = payload
            return
        if not envelope.is_tool_result:
            await self._persist(payload)
            return

        normalized = normalize_tool_result(payload, self._max_bytes)
        key = envelope.call_id or envelope.item_id
        if not key:
            await self._persist(normalized)
            return
        async with self._lock:
            if key not in self._seen_tool_results:
                self._seen_tool_results.add(key)
                persist_now = True
            else:
                persist_now = False
                self._pending[key] = normalized
                if key not in self._timers:
                    self._timers[key] = asyncio.create_task(self._flush_after_window(key))
        if persist_now:
            await self._persist(normalized)

    async def flush(self) -> None:
        async with self._lock:
            timers = tuple(self._timers.values())
            self._timers.clear()
            keys = tuple(self._pending)
        for timer in timers:
            timer.cancel()
        if timers:
            await asyncio.gather(*timers, return_exceptions=True)
        for key in keys:
            async with self._lock:
                payload = self._pending.get(key)
            if payload is None:
                continue
            await self._persist(payload)
            async with self._lock:
                if self._pending.get(key) is payload:
                    self._pending.pop(key, None)

    async def discard(self) -> None:
        """Cancel local timers during process shutdown; PostgreSQL remains authoritative."""
        async with self._lock:
            timers = tuple(self._timers.values())
            self._timers.clear()
            self._pending.clear()
        for timer in timers:
            timer.cancel()
        if timers:
            await asyncio.gather(*timers, return_exceptions=True)

    async def _flush_after_window(self, key: str) -> None:
        try:
            await asyncio.sleep(self._window_seconds)
            async with self._lock:
                payload = self._pending.get(key)
            if payload is not None:
                await self._persist(payload)
            async with self._lock:
                if self._pending.get(key) is payload:
                    self._pending.pop(key, None)
                self._timers.pop(key, None)
        except asyncio.CancelledError:
            raise
        except Exception:
            async with self._lock:
                self._timers.pop(key, None)
            logger.exception(
                "tool result snapshot flush failed session=%s turn=%s",
                self._session_id,
                self._turn_id,
            )

    async def _persist(self, payload: dict[str, object]) -> None:
        event = await self._service.persist_runtime_event(self._session_id, self._turn_id, payload)
        await publish_event(self._bus, event)


async def publish_event(bus: SessionEventBus, event: UniversalEvent) -> None:
    await publish_notification(
        bus,
        SessionEventNotification(event.session_id, event.sequence, event.payload),
    )


async def publish_notification(
    bus: SessionEventBus, notification: SessionEventNotification
) -> None:
    try:
        await bus.publish(notification)
    except Exception:
        logger.exception(
            "Session Event Bus publish failed session=%s sequence=%s",
            notification.session_id,
            notification.sequence,
        )
