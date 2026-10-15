"""Docker-native ownership fixture; real Agent routes/services, deterministic model.

The container receives identity, database and Broker settings from the production
creation service. Native identity is resolved through the real Gateway/Controller.
"""

from __future__ import annotations

import asyncio
import os
from pathlib import Path

import uvicorn

from adapter.config.config import load_agent_config
from adapter.infra.migrations import migrate_database
from scripts.ownership_pipeline_server import application


def main() -> None:
    os.environ["MEDIA_AGENTS_TEST_DB_URL"] = os.environ["MEDIA_AGENTS_DB_URL"]
    os.environ["MEDIA_AGENTS_TEST_RABBITMQ_URL"] = os.environ["MEDIA_AGENTS_RABBITMQ_URL"]
    os.environ["TEST_RUNTIME_RELEASE"] = "/workspace/release"
    cfg = load_agent_config(Path(__file__).parents[1] / "adapter/config/base.yaml")
    cfg.agent.type = "codex"
    asyncio.run(migrate_database(cfg.database.url))
    uvicorn.run(
        application("0", configuration=cfg),
        host="0.0.0.0",
        port=cfg.agent.port,
        log_level="warning",
    )


if __name__ == "__main__":
    main()
