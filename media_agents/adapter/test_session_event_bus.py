from __future__ import annotations

import asyncio
import os

import pytest

from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification
from adapter.session_event_bus import RedisSessionEventBus


@pytest.mark.asyncio
async def test_redis_bus_fans_out_to_independent_subscribers() -> None:
    redis_url = os.environ.get("MEDIA_AGENTS_TEST_REDIS_URL")
    if not redis_url:
        pytest.skip("MEDIA_AGENTS_TEST_REDIS_URL is required for Redis integration")
    first_bus = RedisSessionEventBus(redis_url, "media-agents:test-events")
    second_bus = RedisSessionEventBus(redis_url, "media-agents:test-events")

    async def receive(bus: RedisSessionEventBus) -> SessionEventNotification:
        async for value in bus.subscribe("session-redis"):
            return value
        raise AssertionError("subscription stopped")

    first = asyncio.create_task(receive(first_bus))
    second = asyncio.create_task(receive(second_bus))
    await asyncio.sleep(0.1)
    expected = SessionEventNotification(
        "session-redis", 12, {"type": "item.completed", "value": "fan-out"}
    )
    try:
        await first_bus.publish(expected)
        assert await asyncio.wait_for(first, timeout=2) == expected
        assert await asyncio.wait_for(second, timeout=2) == expected
    finally:
        first.cancel()
        second.cancel()
        await asyncio.gather(first, second, return_exceptions=True)
        await first_bus.close()
        await second_bus.close()
