"""RabbitMQ notification adapter; reconciling confirmed choices repairs lost hints."""

from __future__ import annotations

import asyncio
import json
import logging
from datetime import datetime

import aio_pika
from aio_pika.abc import AbstractIncomingMessage, AbstractRobustConnection

from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow

logger = logging.getLogger(__name__)


class MetadataNotifications:
    def __init__(
        self, url: str, exchange: str, workspace_id: str, config_id: str, flow: MetadataUpdateFlow
    ) -> None:
        self._url, self._exchange = url, exchange
        self._workspace_id, self._config_id = workspace_id, config_id
        self._flow = flow
        self._inflight: set[asyncio.Task[None]] = set()
        self.ready = asyncio.Event()
        self.received_events = 0
        self._connection: AbstractRobustConnection | None = None

    async def run(self) -> None:
        while True:
            try:
                connection = await aio_pika.connect_robust(
                    self._url, timeout=3, reconnect_interval=1
                )
                self._connection = connection
                channel = await connection.channel()
                await channel.set_qos(prefetch_count=1)
                exchange = await channel.declare_exchange(
                    self._exchange, aio_pika.ExchangeType.TOPIC, durable=True
                )
                queue = await channel.declare_queue(
                    "",
                    exclusive=True,
                    auto_delete=True,
                    arguments={
                        "x-max-length": 1000,
                        "x-message-ttl": 300000,
                        "x-queue-type": "classic",
                    },
                )
                key = self._workspace_id.encode().hex() + "." + self._config_id.encode().hex()
                await queue.bind(exchange, routing_key=key)
                await queue.consume(self._receive)
                self.ready.set()
                connection.reconnect_callbacks.add(self._on_reconnect)
                self._flow.wake()
                await asyncio.Future[None]()
            except asyncio.CancelledError:
                raise
            except Exception:
                logger.warning("metadata notification connection deferred")
                await asyncio.sleep(3)
            finally:
                callbacks = tuple(self._inflight)
                for callback in callbacks:
                    callback.cancel()
                await asyncio.gather(*callbacks, return_exceptions=True)
                if self._connection is not None:
                    await self._connection.close()
                    self._connection = None

    async def _on_reconnect(self, connection: object) -> None:
        self._flow.wake()

    async def _receive(self, message: AbstractIncomingMessage) -> None:
        task = asyncio.current_task()
        if task is not None:
            self._inflight.add(task)
        try:
            value = json.loads(message.body)
            if (
                not isinstance(value, dict)
                or type(value.get("schema_version")) is not int
                or value.get("schema_version") != 1
                or type(value.get("data_version")) is not int
                or value.get("data_version") != 1
                or value.get("source") != "media_control"
                or value.get("type") != "agent.metadata.update_requested"
                or not isinstance(value.get("event_id"), str)
                or not value.get("event_id")
                or value.get("event_id") != message.message_id
                or not isinstance(value.get("scope"), dict)
                or value["scope"].get("workspace_id") != self._workspace_id
                or value["scope"].get("agent_config_id") != self._config_id
            ):
                logger.warning("metadata protocol frame isolated")
                return
            occurred_at = value.get("occurred_at")
            if (
                not isinstance(occurred_at, str)
                or datetime.fromisoformat(occurred_at).tzinfo is None
            ):
                logger.warning("metadata protocol timestamp isolated")
                return
            data = value.get("data")
            if (
                not isinstance(data, dict)
                or type(data.get("selection_revision")) is not int
                or data["selection_revision"] <= 0
                or not isinstance(data.get("target_context_version"), str)
                or not data["target_context_version"]
            ):
                logger.warning("metadata protocol data isolated")
                return
            self.received_events += 1
            # changes are advisory, including future unknown kinds.
            await self._flow.reconcile(
                revision=data["selection_revision"], target=data["target_context_version"]
            )
        except Exception:
            logger.warning("metadata notification processing deferred")
            asyncio.get_running_loop().call_later(3, self._flow.wake)
        finally:
            if task is not None:
                self._inflight.discard(task)
            try:
                await message.ack()
            except aio_pika.exceptions.AMQPError:
                self._flow.wake()  # Broker ACK is separate from the persisted application fact.
