from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from typing import Protocol

from adapter.biz.domain.runtime_metadata.entity import InvalidMetadata, MetadataGateway, Snapshot

logger = logging.getLogger(__name__)


class ApplicationStore(Protocol):
    configuration_id: str

    def marker(self) -> dict[str, object]: ...
    def prepare(self, snapshot: Snapshot, artifacts: dict[str, bytes]) -> Path: ...
    def record(
        self, snapshot: Snapshot, status: str, *, artifact: Path | None = None, error_code: str = ""
    ) -> None: ...
    def resolve(self, target: str) -> Path | None: ...
    def pending(self) -> list[dict[str, object]]: ...
    def reported(self, report_id: str) -> None: ...


class MetadataUpdateFlow:
    def __init__(self, gateway: MetadataGateway, store: ApplicationStore, agent_type: str) -> None:
        self._gateway = gateway
        self._store = store
        self._agent_type = agent_type
        self._lock = asyncio.Lock()
        self._wake = asyncio.Event()

    async def reconcile(self, *, revision: int = 0, target: str = "") -> None:
        async with self._lock:
            await self._report_pending()
            for _ in range(3):
                try:
                    snapshot = await self._gateway.fetch(selection_revision=revision, target=target)
                except InvalidMetadata as error:
                    if error.snapshot is not None:
                        self._store.record(
                            error.snapshot,
                            "incompatible",
                            error_code="metadata_schema_incompatible",
                        )
                        await self._report_pending()
                        return
                    raise
                if snapshot is None:
                    return
                marker = self._store.marker()
                applied_revision = marker.get("selection_revision", 0)
                if type(applied_revision) is not int:
                    raise InvalidMetadata("invalid_marker_revision")
                if snapshot.selection_revision < applied_revision:
                    return
                if marker.get("target_context_version") == snapshot.target_context_version:
                    return  # Result retry must not reinstall successful metadata.
                if snapshot.agent_type != self._agent_type:
                    self._store.record(snapshot, "incompatible", error_code="runtime_type_mismatch")
                    await self._report_pending()
                    return
                try:
                    artifacts = {}
                    for resource in snapshot.resources:
                        if resource.kind == "media_control.skill":
                            artifacts[resource.checksum] = await self._gateway.artifact(
                                snapshot.target_context_version, resource.checksum
                            )
                    directory = self._store.prepare(snapshot, artifacts)
                except (InvalidMetadata, ValueError, OSError):
                    self._store.record(snapshot, "failed", error_code="metadata_prepare_failed")
                    await self._report_pending()
                    return
                current = await self._gateway.fetch()
                if (
                    current is None
                    or current.target_context_version != snapshot.target_context_version
                ):
                    self._store.record(snapshot, "superseded")
                    await self._report_pending()
                    revision, target = 0, ""
                    continue
                self._store.record(snapshot, "applied", artifact=directory)
                await self._report_pending()
                return

    async def _report_pending(self) -> None:
        for report in self._store.pending():
            try:
                await self._gateway.report(report)
            except Exception:
                logger.warning(
                    "metadata result delivery deferred report_id=%s", report.get("report_id")
                )
                continue
            report_id = report.get("report_id")
            if not isinstance(report_id, str):
                raise InvalidMetadata("invalid_report_id")
            self._store.reported(report_id)

    def wake(self) -> None:
        self._wake.set()

    async def run(self) -> None:
        while True:
            self._wake.clear()
            try:
                await self.reconcile()
            except Exception:
                logger.warning("confirmed metadata reconciliation deferred")
            try:
                await asyncio.wait_for(self._wake.wait(), timeout=60)
            except TimeoutError:
                pass

    async def resolve_session(self, context_ref: str) -> Path | None:
        if not context_ref.startswith("metadata:"):
            return None
        parts = context_ref.split(":", 2)
        if len(parts) != 3 or parts[1] != self._store.configuration_id or not parts[2]:
            raise InvalidMetadata("invalid_session_context")
        target = parts[2]
        async with self._lock:
            existing = self._store.resolve(target)
            if existing is not None:
                return existing
            snapshot = await self._gateway.fetch(target=target, for_session=True)
            if snapshot is None or snapshot.target_context_version != target:
                raise InvalidMetadata("session_context_unavailable")
            artifacts = {}
            for resource in snapshot.resources:
                if resource.kind == "media_control.skill":
                    artifacts[resource.checksum] = await self._gateway.artifact(
                        target, resource.checksum
                    )
            return self._store.prepare(snapshot, artifacts)
