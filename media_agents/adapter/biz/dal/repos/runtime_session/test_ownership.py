from __future__ import annotations

import asyncio
import os
from uuid import uuid4

import pytest
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.runtime_session.entity import SessionStatus, TurnStatus
from adapter.biz.domain.runtime_session.errors import StaleExecutionOwner, TransferConflict
from adapter.biz.domain.runtime_session.ownership import InstanceScope
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.infra.migrations import migrate_database
from sqlalchemy import text
from sqlalchemy.exc import DBAPIError


@pytest.mark.asyncio
async def test_real_postgres_drain_transfer_fifo_and_fencing() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "ownership-" + str(uuid4())
    services = []
    for index in range(3):
        repo = SqlRuntimeSessionRepository()
        scope = InstanceScope(workspace, "config", str(index), "boot" + str(index))
        service = RuntimeSessionService(TxRunner(factory), repo, scope, repo)
        await service.initialize_instance()
        services.append(service)
    source, target, competitor = services
    session_id = str(uuid4())
    await source.register(RegisterRuntimeSession(session_id, 7, "fixed:v7", workspace, "config"))
    try:
        turns = [
            await source.submit_turn(SubmitTurn(session_id, str(i), 7, {"message": str(i)}))
            for i in range(3)
        ]
        await source.attach_runtime(session_id, turns[0].turn_id, "persisted-native-handle")
        other = str(uuid4())
        await target.register(RegisterRuntimeSession(other, 7, "fixed:v7", workspace, "config"))
        independent = await target.submit_turn(
            SubmitTurn(other, "independent", 7, {"message": "hi"})
        )
        assert len(await source.running_turns()) == 1
        assert len(await target.running_turns()) == 1
        drain = await source.begin_drain("drain-one")
        assert await source.begin_drain("drain-one") == drain
        finalized = await source.finalize_turn(session_id, turns[0].turn_id, TurnStatus.COMPLETED)
        assert finalized.next_turn is None
        assert (await source.get_turn(turns[1].turn_id)).status is TurnStatus.QUEUED
        attempts = await asyncio.gather(
            target.transfer(session_id, "move-a", "0", "boot0", 1),
            competitor.transfer(session_id, "move-b", "0", "boot0", 1),
            return_exceptions=True,
        )
        assert sum(isinstance(result, TransferConflict) for result in attempts) == 1
        winner = target if not isinstance(attempts[0], BaseException) else competitor
        operation = "move-a" if winner is target else "move-b"
        moved, _ = await winner.transfer(session_id, operation, "0", "boot0", 1)
        assert moved.execution_epoch == 2 and moved.runtime_session_id == "persisted-native-handle"
        assert moved.context_ref == "fixed:v7" and moved.context_version == 7
        with pytest.raises(StaleExecutionOwner):
            await source.persist_runtime_event(
                session_id, turns[1].turn_id, {"type": "session.started", "session_id": "native"}
            )
        adopted = await winner.adopt(session_id)
        assert adopted is not None and adopted.turn.turn_id == turns[1].turn_id
        assert adopted.turn.execution_epoch == 2
        next_result = await winner.finalize_turn(session_id, turns[1].turn_id, TurnStatus.COMPLETED)
        assert next_result.next_turn is not None
        assert next_result.next_turn.turn_id == turns[2].turn_id
        assert (await target.get_turn(independent.turn_id)).status is TurnStatus.RUNNING
        async with engine.begin() as connection:
            with pytest.raises(DBAPIError, match="stale_execution_owner"):
                await connection.execute(
                    text(
                        "UPDATE media_agents.runtime_sessions SET chat_status='human' "
                        "WHERE session_id=:id"
                    ),
                    {"id": session_id},
                )
    finally:
        await engine.dispose()


@pytest.mark.asyncio
async def test_real_postgres_unknown_running_transfer_preserves_paused_queue() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "unknown-" + str(uuid4())
    repo = SqlRuntimeSessionRepository()
    old = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "old"), repo
    )
    target = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "1", "new"), repo
    )
    await old.initialize_instance()
    await target.initialize_instance()
    session_id = str(uuid4())
    try:
        await old.register(RegisterRuntimeSession(session_id, 2, "fixed:2", workspace, "config"))
        running = await old.submit_turn(SubmitTurn(session_id, "first", 2, {"message": "first"}))
        queued = await old.submit_turn(SubmitTurn(session_id, "next", 2, {"message": "next"}))
        with pytest.raises(TransferConflict):
            await target.transfer(session_id, "migration", "0", "old", 1)
        moved, events = await target.transfer(
            session_id, "migration", "0", "old", 1, termination_confirmed=True
        )
        assert moved.status is SessionStatus.PAUSED and events
        assert (
            await target.get_turn(running.turn_id)
        ).error_class == "owner_terminated_outcome_unknown"
        assert await target.adopt(session_id) is None
        resumed = await target.adopt(session_id, resume=True, operation_id="resume-one")
        assert resumed is not None and resumed.turn.turn_id == queued.turn_id
        assert (await target.get_turn(running.turn_id)).status is TurnStatus.FAILED
        assert await target.adopt(session_id, resume=True, operation_id="noop-running") is None
        await target.finalize_turn(session_id, queued.turn_id, TurnStatus.FAILED)
        assert await target.adopt(session_id, resume=True, operation_id="noop-running") is None
        assert (await target.get(session_id)).status is SessionStatus.PAUSED
        another = await target.adopt(session_id, resume=True, operation_id="resume-one")
        assert another is None
        assert (await target.get(session_id)).status is SessionStatus.PAUSED
        resumed_again = await target.adopt(session_id, resume=True, operation_id="resume-two")
        assert resumed_again is None
        new_running = await target.submit_turn(
            SubmitTurn(session_id, "third", 2, {"message": "third"})
        )
        # A new boot does not scan/finalize a different incarnation's running Turn.
        replacement = RuntimeSessionService(
            TxRunner(factory), repo, InstanceScope(workspace, "config", "1", "replacement"), repo
        )
        await replacement.initialize_instance()
        assert await replacement.running_turns() == []
        with pytest.raises(StaleExecutionOwner):
            await target.finalize_turn(session_id, new_running.turn_id, TurnStatus.COMPLETED)
        assert (await replacement.get_turn(new_running.turn_id)).status is TurnStatus.RUNNING
    finally:
        await engine.dispose()


@pytest.mark.asyncio
async def test_real_postgres_session_handoff_does_not_drain_replica() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "handoff-" + str(uuid4())
    repo = SqlRuntimeSessionRepository()
    source = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "a"), repo
    )
    target = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "1", "b"), repo
    )
    await source.initialize_instance()
    await target.initialize_instance()
    moving, independent = str(uuid4()), str(uuid4())
    try:
        for session_id in (moving, independent):
            await source.register(
                RegisterRuntimeSession(session_id, 1, "fixed:1", workspace, "config")
            )
        running = await source.submit_turn(SubmitTurn(moving, "first", 1, {"message": "first"}))
        queued = await source.submit_turn(SubmitTurn(moving, "second", 1, {"message": "second"}))
        barrier = await source.quiesce(moving, "move-one")
        assert await source.quiesce(moving, "move-one") == barrier
        with pytest.raises(TransferConflict):
            await source.quiesce(moving, "different-move")
        finished = await source.finalize_turn(moving, running.turn_id, TurnStatus.COMPLETED)
        assert finished.next_turn is None
        assert await source.adopt(moving) is None
        other = await source.submit_turn(SubmitTurn(independent, "first", 1, {"message": "other"}))
        assert other.status is TurnStatus.RUNNING
        assert (await source.drain_state())[0].state == "active"
        moved, _ = await target.transfer(moving, "move-one", "0", "a", 1)
        assert not moved.handoff_id
        adopted = await target.adopt(moving)
        assert adopted is not None and adopted.turn.turn_id == queued.turn_id
        assert adopted.turn.execution_epoch == 2
    finally:
        await engine.dispose()


@pytest.mark.asyncio
async def test_real_postgres_late_initial_registration_cannot_replace_transfer_creation() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "initial-" + str(uuid4())
    repo = SqlRuntimeSessionRepository()
    source = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "a"), repo
    )
    target = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "1", "b"), repo
    )
    await source.initialize_instance()
    await target.initialize_instance()
    session_id = str(uuid4())
    try:
        created = await target.register(
            RegisterRuntimeSession(session_id, 7, "fixed:7", workspace, "config", 2, "initial-move")
        )
        assert created.execution_epoch == 2 and created.last_transfer_id == "initial-move"
        with pytest.raises(StaleExecutionOwner):
            await source.register(
                RegisterRuntimeSession(session_id, 7, "fixed:7", workspace, "config")
            )
        assert (
            await target.register(
                RegisterRuntimeSession(
                    session_id, 7, "fixed:7", workspace, "config", 2, "initial-move"
                )
            )
            == created
        )
        assert not await target.running_turns()
    finally:
        await engine.dispose()


@pytest.mark.asyncio
async def test_real_postgres_legacy_idle_assignment_and_running_refusal() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "legacy-" + str(uuid4())
    repo = SqlRuntimeSessionRepository()
    legacy = RuntimeSessionService(TxRunner(factory), repo)
    managed = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "new"), repo
    )
    await managed.initialize_instance()
    idle, active = str(uuid4()), str(uuid4())
    try:
        await legacy.register(RegisterRuntimeSession(idle, 7, "fixed:7"))
        assigned = await managed.register(
            RegisterRuntimeSession(idle, 7, "fixed:7", workspace, "config")
        )
        assert assigned.owner_incarnation_id == "new" and assigned.execution_epoch == 1
        with pytest.raises(StaleExecutionOwner):
            await legacy.submit_turn(SubmitTurn(idle, "legacy", 7, {"message": "legacy"}))
        await legacy.register(RegisterRuntimeSession(active, 7, "fixed:7"))
        await legacy.submit_turn(SubmitTurn(active, "running", 7, {"message": "legacy-running"}))
        with pytest.raises(TransferConflict):
            await managed.register(
                RegisterRuntimeSession(active, 7, "fixed:7", workspace, "config")
            )
        async with engine.begin() as connection:
            for assignment in ("owner_instance_id='other'", "context_version=8"):
                with pytest.raises(DBAPIError):
                    async with connection.begin_nested():
                        await connection.execute(
                            text(
                                "SELECT set_config('media_agent.workspace',:workspace,true), "
                                "set_config('media_agent.config','config',true), "
                                "set_config('media_agent.replica','0',true), "
                                "set_config('media_agent.incarnation','new',true), "
                                "set_config('media_agent.epoch','1',true)"
                            ),
                            {"workspace": workspace},
                        )
                        await connection.execute(
                            text(
                                "UPDATE media_agents.runtime_sessions SET "
                                + assignment
                                + " WHERE session_id=:id"
                            ),
                            {"id": idle},
                        )
    finally:
        await engine.dispose()


@pytest.mark.asyncio
async def test_registered_new_incarnation_reactivates_retired_ordinal() -> None:
    url = os.environ.get("MEDIA_AGENTS_TEST_DB_URL")
    if not url:
        pytest.skip("real PostgreSQL required")
    await migrate_database(url)
    engine, factory = create_database(url)
    workspace = "retirement-" + str(uuid4())
    repo = SqlRuntimeSessionRepository()
    previous = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "previous"), repo
    )
    replacement = RuntimeSessionService(
        TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "replacement"), repo
    )
    try:
        await previous.initialize_instance("active")
        await previous.begin_drain("retirement")
        await previous.initialize_instance("active")
        admission, _ = await previous.drain_state()
        assert admission.state == "draining"
        await replacement.initialize_instance("active")
        admission, _ = await replacement.drain_state()
        assert admission.state == "active"
        assert admission.operation_id == ""
        await replacement.begin_drain("next-scale")
        conservative = RuntimeSessionService(
            TxRunner(factory), repo, InstanceScope(workspace, "config", "0", "unverified"), repo
        )
        await conservative.initialize_instance()
        admission, _ = await conservative.drain_state()
        assert admission.state == "draining"
        assert admission.operation_id == "next-scale"
    finally:
        await engine.dispose()
