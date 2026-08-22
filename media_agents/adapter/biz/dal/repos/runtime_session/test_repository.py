from __future__ import annotations

import os
import uuid

import pytest
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.runtime_session.errors import ContextVersionConflict
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.infra.migrations import migrate_database
from sqlalchemy import text


@pytest.mark.asyncio
async def test_postgresql_repository_persists_idempotent_registration() -> None:
    database_url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not database_url:
        pytest.skip("MEDIA_AGENTS_TEST_DB_URL is required for PostgreSQL DAL integration")
    await migrate_database(database_url)
    engine, session_factory = create_database(database_url)
    service = RuntimeSessionService(TxRunner(session_factory), SqlRuntimeSessionRepository())
    session_id = f"dal-{uuid.uuid4()}"
    command = RegisterRuntimeSession(session_id, 7, "dal:context:v7")
    try:
        created = await service.register(command)
        repeated = await service.register(command)
        fetched = await service.get(session_id)

        assert repeated == created
        assert fetched == created
        with pytest.raises(ContextVersionConflict):
            await service.register(RegisterRuntimeSession(session_id, 8, "dal:context:v8"))
    finally:
        async with engine.begin() as connection:
            await connection.execute(
                text("DELETE FROM media_agents.runtime_sessions WHERE session_id = :session_id"),
                {"session_id": session_id},
            )
        await engine.dispose()
