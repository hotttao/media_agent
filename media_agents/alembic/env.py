from __future__ import annotations

import asyncio
from logging.config import fileConfig

from sqlalchemy import pool, text
from sqlalchemy.engine import Connection
from sqlalchemy.ext.asyncio import async_engine_from_config

from adapter.biz.dal.models import Base
from alembic import context

config = context.config
if config.config_file_name is not None:
    fileConfig(config.config_file_name, disable_existing_loggers=False)
target_metadata = Base.metadata


def run_migrations_offline() -> None:
    context.configure(
        url=config.get_main_option("sqlalchemy.url"),
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
    )
    with context.begin_transaction():
        context.run_migrations()


def _run_migrations(connection: Connection) -> None:
    connection.execute(text("CREATE SCHEMA IF NOT EXISTS media_agents"))
    context.configure(
        connection=connection,
        target_metadata=target_metadata,
        include_schemas=True,
        version_table_schema="media_agents",
    )
    with context.begin_transaction():
        context.run_migrations()


async def run_migrations_online() -> None:
    configuration = config.get_section(config.config_ini_section, {})
    connectable = async_engine_from_config(
        configuration,
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    async with connectable.begin() as connection:
        await connection.run_sync(_run_migrations)
    await connectable.dispose()


if context.is_offline_mode():
    run_migrations_offline()
else:
    asyncio.run(run_migrations_online())
