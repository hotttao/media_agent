from __future__ import annotations

import hashlib
import io
import json
import zipfile
from dataclasses import replace
from pathlib import Path

import pytest

from adapter.biz.domain.runtime_metadata.entity import InvalidMetadata, Resource, Snapshot
from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow
from adapter.metadata_artifacts import MetadataArtifacts
from adapter.runtime_metadata_files import claude_args, codex_home


def skill_zip(text: str, *, name: str = "SKILL.md") -> bytes:
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as archive:
        archive.writestr(name, "---\nname: skill\ndescription: Metadata test skill\n---\n" + text)
    return stream.getvalue()


def snapshot(
    target: str, revision: int, text: str, artifact: bytes, *, agent_type: str = "codex"
) -> Snapshot:
    return Snapshot(
        "workspace",
        "config",
        revision,
        target,
        revision,
        agent_type,
        text,
        "Platform " + target,
        "1",
        (
            Resource(
                "media_control.skill",
                "skill",
                revision,
                "Skill",
                hashlib.sha256(artifact).hexdigest(),
            ),
        ),
    )


class Gateway:
    def __init__(self, value: Snapshot, artifacts: dict[str, bytes]) -> None:
        self.current = value
        self.history = {value.target_context_version: value}
        self.artifacts = artifacts
        self.reports: list[dict[str, object]] = []
        self.downloads = 0
        self.fail_report = False
        self.change_on_recheck: Snapshot | None = None

    async def fetch(
        self, *, selection_revision: int = 0, target: str = "", for_session: bool = False
    ) -> Snapshot:
        if not target and self.change_on_recheck is not None:
            self.current = self.change_on_recheck
            self.history[self.current.target_context_version] = self.current
            self.change_on_recheck = None
        if for_session:
            return self.history[target]
        if target and target != self.current.target_context_version:
            return replace(self.current, status="superseded")
        return self.current

    async def artifact(self, target: str, checksum: str) -> bytes:
        self.downloads += 1
        return self.artifacts[checksum]

    async def report(self, result: dict[str, object]) -> None:
        if self.fail_report:
            raise ConnectionError("report unavailable")
        self.reports.append(result)


@pytest.mark.asyncio
async def test_update_and_explicit_rollback_preserve_old_session_files(tmp_path: Path) -> None:
    one, two = skill_zip("old skill"), skill_zip("new skill")
    first, second = snapshot("v-one", 2, "old prompt", one), snapshot("v-two", 3, "new prompt", two)
    gateway = Gateway(
        first, {hashlib.sha256(one).hexdigest(): one, hashlib.sha256(two).hexdigest(): two}
    )
    store = MetadataArtifacts(tmp_path, "config", "0", "boot0")
    flow = MetadataUpdateFlow(gateway, store, "codex")
    await flow.reconcile()
    old_bundle = await flow.resolve_session("metadata:config:v-one")
    assert old_bundle is not None
    old_home = codex_home(old_bundle, "old-session", tmp_path, tmp_path / "auth")
    gateway.current = second
    gateway.history[second.target_context_version] = second
    await flow.reconcile()
    new_bundle = await flow.resolve_session("metadata:config:v-two")
    assert new_bundle is not None and new_bundle != old_bundle
    new_home = codex_home(new_bundle, "new-session", tmp_path, tmp_path / "auth")
    assert "old prompt" in (old_home / "config.toml").read_text()
    assert "new prompt" in (new_home / "config.toml").read_text()
    assert (old_home / "skills/skill/SKILL.md").read_text().endswith("old skill")
    assert (new_home / "skills/skill/SKILL.md").read_text().endswith("new skill")
    assert not (tmp_path / "AGENTS.md").exists()
    rollback = replace(
        first, target_context_version="v-rollback", selection_revision=4, context_version=4
    )
    gateway.current = rollback
    gateway.history[rollback.target_context_version] = rollback
    await flow.reconcile(revision=2, target="v-one")
    assert store.marker()["selection_revision"] == 4
    assert store.marker()["target_context_version"] == "v-rollback"
    assert await flow.resolve_session("metadata:config:v-two") == new_bundle
    with pytest.raises(InvalidMetadata):
        await flow.resolve_session("metadata:other:v-one")
    with pytest.raises(InvalidMetadata):
        codex_home(new_bundle, "old-session", tmp_path, tmp_path / "auth")


@pytest.mark.asyncio
async def test_report_retry_does_not_redownload_or_reinstall(tmp_path: Path) -> None:
    content = skill_zip("skill")
    value = snapshot("version", 2, "prompt", content)
    gateway = Gateway(value, {value.resources[0].checksum: content})
    gateway.fail_report = True
    store = MetadataArtifacts(tmp_path, "config", "0", "boot0")
    await MetadataUpdateFlow(gateway, store, "codex").reconcile()
    pending = store.pending()
    assert len(pending) == 1 and gateway.downloads == 1
    report_id = pending[0]["report_id"]
    gateway.fail_report = False
    # New workflow models a process restart with the same persisted application marker.
    await MetadataUpdateFlow(gateway, store, "codex").reconcile()
    assert gateway.downloads == 1 and not store.pending()
    assert gateway.reports[0]["report_id"] == report_id


@pytest.mark.asyncio
async def test_failed_install_preserves_last_good_and_rejects_archive_escape(
    tmp_path: Path,
) -> None:
    good, bad = skill_zip("old"), skill_zip("escape", name="../outside")
    first, second = snapshot("good", 2, "old", good), snapshot("bad", 3, "new", bad)
    gateway = Gateway(first, {first.resources[0].checksum: good, second.resources[0].checksum: bad})
    store = MetadataArtifacts(tmp_path, "config", "0", "boot0")
    flow = MetadataUpdateFlow(gateway, store, "codex")
    await flow.reconcile()
    gateway.current = second
    await flow.reconcile()
    assert store.marker()["target_context_version"] == "good"
    assert gateway.reports[-1]["status"] == "failed"
    assert not (tmp_path / "outside").exists()
    assert store.resolve("bad") is None


@pytest.mark.asyncio
async def test_parallel_instances_share_bundle_and_keep_separate_markers(tmp_path: Path) -> None:
    import asyncio

    content = skill_zip("skill")
    value = snapshot("version", 2, "prompt", content)
    gateway = Gateway(value, {value.resources[0].checksum: content})
    one = MetadataArtifacts(tmp_path, "config", "0", "boot0")
    two = MetadataArtifacts(tmp_path, "config", "1", "boot1")
    await asyncio.gather(
        MetadataUpdateFlow(gateway, one, "codex").reconcile(),
        MetadataUpdateFlow(gateway, two, "codex").reconcile(),
    )
    assert one.resolve("version") == two.resolve("version")
    assert {r["incarnation_id"] for r in gateway.reports} == {"boot0", "boot1"}
    assert len({r["report_id"] for r in gateway.reports}) == 2
    marker_text = json.dumps(one.marker())
    assert "old prompt" not in marker_text and "SKILL.md" not in marker_text


def test_claude_native_config_is_pinned_without_replacing_builtin_prompt(tmp_path: Path) -> None:
    content = skill_zip("skill")
    value = snapshot("claude-v1", 2, "configured", content, agent_type="claude-code")
    store = MetadataArtifacts(tmp_path, "config", "0", "boot0")
    bundle = store.prepare(value, {value.resources[0].checksum: content})
    args = claude_args(bundle)
    assert args[0] == "--append-system-prompt-file"
    assert Path(args[1]).read_text() == "Platform claude-v1\n\nconfigured"
    assert "--plugin-dir" in args
    assert (bundle / "platform_prompt.md").read_text() == "Platform claude-v1"
    assert (bundle / "configured_prompt.md").read_text() == "configured"


def test_memory_descriptor_and_corrupt_bundle_are_verified(tmp_path: Path) -> None:
    content = skill_zip("skill")
    value = snapshot("version", 2, "prompt", content)
    descriptor = json.dumps({"id": "memory", "configuration_ref": {"version": 7}})
    memory = Resource(
        "media_control.memory_store",
        "memory",
        7,
        "Memory",
        hashlib.sha256(descriptor.encode()).hexdigest(),
        content=descriptor,
    )
    value = replace(value, resources=(*value.resources, memory))
    store = MetadataArtifacts(tmp_path, "config", "0", "boot")
    bundle = store.prepare(value, {value.resources[0].checksum: content})
    assert (bundle / "memory/memory.json").read_text() == descriptor
    MetadataArtifacts.verify(bundle)
    (bundle / "skills/skill/SKILL.md").write_text("corrupted")
    with pytest.raises(InvalidMetadata):
        store.prepare(value, {value.resources[0].checksum: content})


@pytest.mark.asyncio
async def test_notice_rejects_boolean_versions_and_accepts_unknown_change_hints(
    tmp_path: Path,
) -> None:
    from typing import Any, cast

    from adapter.metadata_notifications import MetadataNotifications

    class Message:
        def __init__(self, value: dict[str, object]) -> None:
            self.body = json.dumps(value).encode()
            self.message_id = "event"
            self.acked = False

        async def ack(self) -> None:
            self.acked = True

    content = skill_zip("skill")
    value = snapshot("version", 2, "prompt", content)
    gateway = Gateway(value, {value.resources[0].checksum: content})
    flow = MetadataUpdateFlow(gateway, MetadataArtifacts(tmp_path, "config", "0", "boot"), "codex")
    consumer = MetadataNotifications("unused", "metadata", "workspace", "config", flow)
    frame: dict[str, object] = {
        "schema_version": 1,
        "data_version": 1,
        "source": "media_control",
        "type": "agent.metadata.update_requested",
        "occurred_at": "2026-10-12T00:00:00Z",
        "event_id": "event",
        "scope": {"workspace_id": "workspace", "agent_config_id": "config"},
        "data": {
            "selection_revision": 2,
            "target_context_version": "version",
            "changes": [{"kind": "future.resource", "action": "new-action"}],
        },
    }
    malformed = Message({**frame, "schema_version": True})
    await consumer._receive(cast(Any, malformed))
    assert malformed.acked and not gateway.reports
    valid = Message(frame)
    await consumer._receive(cast(Any, valid))
    assert valid.acked and consumer.received_events == 1
    assert gateway.reports[-1]["status"] == "applied"
    bundle = await flow.resolve_session("metadata:config:version")
    assert bundle is not None
    assert (
        json.loads((bundle / "metadata.json").read_text())["platform_prompt_template_version"]
        == value.platform_prompt_template_version
    )
