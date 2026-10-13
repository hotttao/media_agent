from __future__ import annotations

import asyncio
import os
from collections.abc import AsyncGenerator

import pytest

from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification
from adapter.session_event_bus import RabbitMQSessionEventBus


async def start_receiver(
    bus: RabbitMQSessionEventBus, session_id: str, count: int
) -> tuple[AsyncGenerator[SessionEventNotification, None], asyncio.Task[SessionEventNotification]]:
    iterator = bus.subscribe(session_id)
    task = asyncio.create_task(anext(iterator))
    async with asyncio.timeout(5):
        while len(bus._subscribers.get(session_id, set())) < count:
            if task.done():
                await task
            await asyncio.sleep(0.01)
    return iterator, task


@pytest.mark.asyncio
async def test_rabbit_bus_fans_out_without_per_subscriber_queues() -> None:
    broker_url = os.environ.get("MEDIA_AGENTS_TEST_RABBITMQ_URL")
    if not broker_url:
        pytest.skip("MEDIA_AGENTS_TEST_RABBITMQ_URL is required for RabbitMQ integration")
    first_bus = RabbitMQSessionEventBus(broker_url, "test.python.events")
    second_bus = RabbitMQSessionEventBus(broker_url, "test.python.events")
    publisher = RabbitMQSessionEventBus(broker_url, "test.python.events")
    session_id = "session.*.#.会话"
    receivers = []
    try:
        receivers.append(await start_receiver(first_bus, session_id, 1))
        receivers.append(await start_receiver(first_bus, session_id, 2))
        receivers.append(await start_receiver(second_bus, session_id, 1))
        first_queue = first_bus._queue
        assert first_queue is not None and second_bus._queue is not None
        assert first_queue.name != second_bus._queue.name
        expected = SessionEventNotification(
            session_id, 12, {"type": "item.completed", "value": "中文"}
        )
        await publisher.publish(expected)
        for _, task in receivers:
            assert await asyncio.wait_for(task, timeout=3) == expected
        assert first_bus._queue is first_queue
        assert publisher._queue is None
        assert first_bus._connection is not None
        await first_bus._connection.reconnect()
        assert first_bus._channel is not None
        await first_bus._channel.restore()
        # The existing local iterator must work after robust queue/binding recovery.
        await receivers[0][0].aclose()
        next_local = asyncio.create_task(anext(receivers[1][0]))
        next_other = asyncio.create_task(anext(receivers[2][0]))
        delta = SessionEventNotification(session_id, 0, {"type": "delta"})
        await publisher.publish(delta)
        assert await asyncio.wait_for(next_local, 3) == delta
        assert await asyncio.wait_for(next_other, 3) == delta
    finally:
        for iterator, task in receivers:
            task.cancel()
            await asyncio.gather(task, return_exceptions=True)
            await iterator.aclose()
        await first_bus.close()
        await second_bus.close()
        await publisher.close()


@pytest.mark.asyncio
async def test_rabbit_publish_without_viewers_does_not_fail() -> None:
    broker_url = os.environ.get("MEDIA_AGENTS_TEST_RABBITMQ_URL")
    if not broker_url:
        pytest.skip("MEDIA_AGENTS_TEST_RABBITMQ_URL is required for RabbitMQ integration")
    bus = RabbitMQSessionEventBus(broker_url, "test.python.no-viewers")
    try:
        await bus.publish(SessionEventNotification("absent", 0, {"type": "delta"}))
    finally:
        await bus.close()
