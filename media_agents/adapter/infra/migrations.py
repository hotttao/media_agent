"""Run versioned Agent database migrations without blocking the event loop."""

from __future__ import annotations

import asyncio
from pathlib import Path

from alembic import command
from alembic.config import Config


async def migrate_database(database_url: str) -> None:
    workspace_root = Path(__file__).resolve().parents[2]
    config = Config(str(workspace_root / "alembic.ini"))
    config.set_main_option("script_location", str(workspace_root / "alembic"))
    config.set_main_option("sqlalchemy.url", database_url.replace("%", "%%"))
    await asyncio.to_thread(command.upgrade, config, "head")
