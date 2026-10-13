"""Cross-language integration driver, invoked by the Go Harness test."""

from __future__ import annotations

import asyncio
import os
from pathlib import Path

from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow
from adapter.clients.metadata import GatewayMetadataClient
from adapter.metadata_artifacts import MetadataArtifacts
from adapter.metadata_notifications import MetadataNotifications


async def main() -> None:
    target = os.environ["TEST_METADATA_CONTEXT"]
    tasks: list[asyncio.Task[None]] = []
    clients = []
    stores = []
    flows = []
    notifications_list = []
    try:
        for ordinal in ("0", "1"):
            client = GatewayMetadataClient(
                os.environ["TEST_GATEWAY_URL"],
                "agent-test-token",
                "workspace",
                "config",
                ordinal,
                "boot" + ordinal,
                "instance-" + ordinal + "-test-token",
            )
            store = MetadataArtifacts(
                Path(os.environ["TEST_METADATA_ROOT"]), "config", ordinal, "boot" + ordinal
            )
            flow = MetadataUpdateFlow(client, store, "codex")
            notifications = MetadataNotifications(
                os.environ["MEDIA_AGENTS_TEST_RABBITMQ_URL"],
                os.environ["TEST_METADATA_EXCHANGE"],
                "workspace",
                "config",
                flow,
            )
            clients.append(client)
            stores.append(store)
            flows.append(flow)
            notifications_list.append(notifications)
            tasks.append(asyncio.create_task(notifications.run()))
        async with asyncio.timeout(15):
            for notification in notifications_list:
                await notification.ready.wait()
        Path(os.environ["TEST_METADATA_READY"]).write_text("ready")
        async with asyncio.timeout(15):
            while any(
                store.marker().get("target_context_version") != target or store.pending()
                for store in stores
            ):
                await asyncio.sleep(0.05)
        assert all(notification.received_events > 0 for notification in notifications_list)
        for store in stores:
            bundle = store.resolve(target)
            assert bundle is not None and (bundle / "configured_prompt.md").read_text() == "new"
        assert (
            (stores[0].resolve(target) / "skills/skill/SKILL.md").read_text().endswith("new skill")
        )
        old = await flows[0].resolve_session(
            "metadata:config:" + os.environ["TEST_PREVIOUS_CONTEXT"]
        )
        assert old is not None and (old / "configured_prompt.md").read_text() == "old"
        assert (old / "skills/skill/SKILL.md").read_text().endswith("old skill")
        print(
            "PASS RabbitMQ notifications, two instances, immutable Skills and old Session versions"
        )
    finally:
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        for client in clients:
            await client.close()


if __name__ == "__main__":
    asyncio.run(main())
