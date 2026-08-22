from __future__ import annotations

import asyncio
import os
import uuid

import pytest
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.runtime_session.entity import RuntimeSession, TurnStatus
from adapter.biz.domain.runtime_session.errors import ContextVersionConflict
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.infra.migrations import migrate_database
from sqlalchemy import text


class FailingSubmissionRepository(SqlRuntimeSessionRepository):
    async def save_submission(self, session: RuntimeSession) -> None:
        raise RuntimeError("injected submission failure")


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


@pytest.mark.asyncio
async def test_postgresql_serializes_concurrent_turn_submissions() -> None:
    database_url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not database_url:
        pytest.skip("MEDIA_AGENTS_TEST_DB_URL is required for PostgreSQL DAL integration")
    await migrate_database(database_url)
    engine, session_factory = create_database(database_url)
    service = RuntimeSessionService(TxRunner(session_factory), SqlRuntimeSessionRepository())
    session_id = f"fifo-{uuid.uuid4()}"
    await service.register(RegisterRuntimeSession(session_id, 1, "dal:context:v1"))
    try:
        commands = [
            SubmitTurn(session_id, f"key-{index}", 1, {"text": str(index)}) for index in range(8)
        ]
        turns = await asyncio.gather(*(service.submit_turn(command) for command in commands))
        repeated = await service.submit_turn(commands[0])

        assert repeated.turn_id == turns[0].turn_id
        assert sorted(turn.enqueue_sequence for turn in turns) == list(range(1, 9))
        assert sum(turn.status.value == "running" for turn in turns) == 1
        async with engine.connect() as connection:
            counts = (
                await connection.execute(
                    text(
                        "SELECT (SELECT count(*) FROM media_agents.turns WHERE session_id=:id), "
                        "(SELECT count(*) FROM media_agents.messages WHERE session_id=:id)"
                    ),
                    {"id": session_id},
                )
            ).one()
        assert counts == (8, 8)
        running = next(turn for turn in turns if turn.status.value == "running")
        completed = await service.finalize_turn(session_id, running.turn_id, TurnStatus.COMPLETED)
        assert completed.next_turn is not None
        attached = await service.attach_runtime(
            session_id, completed.next_turn.turn_id, "runtime-dal-1"
        )
        assert attached.runtime_session_id == "runtime-dal-1"
        failed = await service.finalize_turn(
            session_id,
            completed.next_turn.turn_id,
            TurnStatus.FAILED,
            error_class="InjectedFailure",
        )
        assert failed.next_turn is None
        assert (await service.get(session_id)).status.value == "paused"
        events = await service.replay_events(session_id, 0, limit=100)
        assert [event.sequence for event in events] == list(range(1, len(events) + 1))
        assert [
            event.sequence
            for event in await service.replay_events(session_id, events[-2].sequence, limit=100)
        ] == [events[-1].sequence]
    finally:
        async with engine.begin() as connection:
            await connection.execute(
                text("DELETE FROM media_agents.universal_events WHERE session_id=:id"),
                {"id": session_id},
            )
            await connection.execute(
                text("DELETE FROM media_agents.messages WHERE session_id=:id"), {"id": session_id}
            )
            await connection.execute(
                text("DELETE FROM media_agents.turns WHERE session_id=:id"), {"id": session_id}
            )
            await connection.execute(
                text("DELETE FROM media_agents.runtime_sessions WHERE session_id=:id"),
                {"id": session_id},
            )
        await engine.dispose()


@pytest.mark.asyncio
async def test_postgresql_rolls_back_turn_and_message_together() -> None:
    database_url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not database_url:
        pytest.skip("MEDIA_AGENTS_TEST_DB_URL is required for PostgreSQL DAL integration")
    await migrate_database(database_url)
    engine, session_factory = create_database(database_url)
    normal = RuntimeSessionService(TxRunner(session_factory), SqlRuntimeSessionRepository())
    failing = RuntimeSessionService(TxRunner(session_factory), FailingSubmissionRepository())
    session_id = f"rollback-{uuid.uuid4()}"
    await normal.register(RegisterRuntimeSession(session_id, 1, "dal:context:v1"))
    try:
        with pytest.raises(RuntimeError, match="injected"):
            await failing.submit_turn(SubmitTurn(session_id, "key", 1, {"text": "rollback"}))
        async with engine.connect() as connection:
            counts = (
                await connection.execute(
                    text(
                        "SELECT (SELECT count(*) FROM media_agents.turns WHERE session_id=:id), "
                        "(SELECT count(*) FROM media_agents.messages WHERE session_id=:id)"
                    ),
                    {"id": session_id},
                )
            ).one()
        assert counts == (0, 0)
    finally:
        async with engine.begin() as connection:
            await connection.execute(
                text("DELETE FROM media_agents.runtime_sessions WHERE session_id=:id"),
                {"id": session_id},
            )
        await engine.dispose()


@pytest.mark.asyncio
async def test_postgresql_persists_agent_request_and_idempotent_response() -> None:
    database_url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not database_url:
        pytest.skip("MEDIA_AGENTS_TEST_DB_URL is required for PostgreSQL DAL integration")
    await migrate_database(database_url)
    engine, session_factory = create_database(database_url)
    service = RuntimeSessionService(TxRunner(session_factory), SqlRuntimeSessionRepository())
    session_id = f"request-{uuid.uuid4()}"
    await service.register(RegisterRuntimeSession(session_id, 1, "dal:context:v1"))
    turn = await service.submit_turn(SubmitTurn(session_id, "request-key", 1, {"message": "hi"}))
    try:
        opened = await service.open_agent_request(
            session_id,
            turn.turn_id,
            "permission",
            {"options": [{"option_id": "allow-once"}]},
        )
        first = await service.resolve_agent_request(
            session_id,
            turn.turn_id,
            opened.request.request_id,
            {"option_id": "allow-once"},
        )
        repeated = await service.resolve_agent_request(
            session_id,
            turn.turn_id,
            opened.request.request_id,
            {"option_id": "allow-once"},
        )
        assert first.request.status.value == "resolved"
        assert repeated.event is None
        assert [event.event_type for event in await service.replay_events(session_id, 0)][-2:] == [
            "agent.requested",
            "agent.request.resolved",
        ]
    finally:
        async with engine.begin() as connection:
            for table in ("agent_requests", "universal_events", "messages", "turns"):
                await connection.execute(
                    text(f"DELETE FROM media_agents.{table} WHERE session_id=:id"),
                    {"id": session_id},
                )
            await connection.execute(
                text("DELETE FROM media_agents.runtime_sessions WHERE session_id=:id"),
                {"id": session_id},
            )
        await engine.dispose()
