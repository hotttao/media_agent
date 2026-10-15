"""Filesystem adapter: immutable bundles and one atomic instance application marker."""

from __future__ import annotations

import hashlib
import io
import json
import os
import re
import shutil
import stat
import tempfile
import zipfile
from dataclasses import asdict
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5

import yaml

from adapter.biz.domain.runtime_metadata.entity import InvalidMetadata, Snapshot


def _segment(value: str) -> str:
    if re.fullmatch(r"[A-Za-z0-9_-]{1,128}", value) is None:
        raise InvalidMetadata("invalid_metadata_identity")
    return value


def _json(value: object) -> bytes:
    return json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":")).encode()


def _write(path: Path, content: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("wb") as file:
        file.write(content)
        file.flush()
        os.fsync(file.fileno())


def _sync(path: Path) -> None:
    descriptor = os.open(path, os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def marker_revision(marker: dict[str, object]) -> int:
    value = marker.get("selection_revision", 0)
    if type(value) is not int or value < 0:
        raise InvalidMetadata("invalid_marker_revision")
    return value


class MetadataArtifacts:
    def __init__(self, root: Path, config_id: str, replica_id: str, incarnation_id: str) -> None:
        self._root = root / ".metadata" / _segment(config_id)
        self._bundles = self._root / "bundles"
        self._marker = (
            self._root
            / "instances"
            / _segment(replica_id)
            / _segment(incarnation_id)
            / "application.json"
        )
        self.configuration_id = config_id
        self._replica_id = replica_id
        self._incarnation_id = incarnation_id

    def marker(self) -> dict[str, object]:
        if not self._marker.exists():
            return {"selection_revision": 0, "pending_reports": []}
        value = json.loads(self._marker.read_bytes())
        if not isinstance(value, dict) or type(value.get("selection_revision")) is not int:
            raise InvalidMetadata("invalid_application_marker")
        return value

    def _save(self, value: dict[str, object]) -> None:
        self._marker.parent.mkdir(parents=True, exist_ok=True)
        descriptor, name = tempfile.mkstemp(prefix=".application-", dir=self._marker.parent)
        os.close(descriptor)
        staging = Path(name)
        try:
            _write(staging, _json(value))
            os.replace(staging, self._marker)
            _sync(self._marker.parent)
        finally:
            staging.unlink(missing_ok=True)

    def prepare(self, snapshot: Snapshot, artifacts: dict[str, bytes]) -> Path:
        if snapshot.agent_config_id != self.configuration_id:
            raise InvalidMetadata("scope_mismatch")
        snapshot_value = asdict(snapshot)
        snapshot_value.pop("status")
        snapshot_value["metadata_schema_version"] = 1
        digest = hashlib.sha256(_json(snapshot_value)).hexdigest()
        target = self._bundles / _segment(snapshot.target_context_version) / digest
        if target.exists():
            self.verify(target)
            return target
        target.parent.mkdir(parents=True, exist_ok=True)
        staging = Path(tempfile.mkdtemp(prefix=".staging-", dir=target.parent))
        try:
            _write(staging / "configured_prompt.md", snapshot.configured_prompt.encode())
            _write(staging / "platform_prompt.md", snapshot.platform_prompt.encode())
            _write(staging / "metadata.json", _json(snapshot_value))
            if snapshot.agent_type == "claude-code":
                _write(
                    staging / ".claude-plugin" / "plugin.json",
                    _json(
                        {
                            "name": "workspace-metadata",
                            "version": "1.0." + str(snapshot.selection_revision),
                            "description": "Pinned workspace skills",
                            "author": {"name": "Media Agent"},
                        }
                    ),
                )
                _write(
                    staging / "claude" / "system_prompt_append.md",
                    (snapshot.platform_prompt + "\n\n" + snapshot.configured_prompt).encode(),
                )
            for resource in snapshot.resources:
                if resource.kind == "media_control.configured_prompt":
                    if (
                        hashlib.sha256(snapshot.configured_prompt.encode()).hexdigest()
                        != resource.checksum
                    ):
                        raise InvalidMetadata("prompt_checksum_mismatch")
                elif resource.kind == "media_control.memory_store":
                    if hashlib.sha256(resource.content.encode()).hexdigest() != resource.checksum:
                        raise InvalidMetadata("memory_checksum_mismatch")
                    _write(
                        staging / "memory" / (_segment(resource.resource_id) + ".json"),
                        resource.content.encode(),
                    )
                elif resource.kind == "media_control.skill":
                    content = artifacts.get(resource.checksum)
                    if content is None or hashlib.sha256(content).hexdigest() != resource.checksum:
                        raise InvalidMetadata("skill_checksum_mismatch")
                    self._unpack(content, staging / "skills" / _segment(resource.resource_id))
            manifest = {
                str(path.relative_to(staging)): hashlib.sha256(path.read_bytes()).hexdigest()
                for path in staging.rglob("*")
                if path.is_file()
            }
            _write(staging / "checksums.json", _json(manifest))
            for directory in sorted(
                (p for p in staging.rglob("*") if p.is_dir()),
                key=lambda p: len(p.parts),
                reverse=True,
            ):
                _sync(directory)
            _sync(staging)
            try:
                os.rename(staging, target)
            except OSError:
                if not target.exists():
                    raise
                self.verify(target)
            _sync(target.parent)
            return target
        except zipfile.BadZipFile as error:
            raise InvalidMetadata("invalid_skill_archive") from error
        finally:
            if staging.exists():
                shutil.rmtree(staging)

    @staticmethod
    def _unpack(content: bytes, target: Path) -> None:
        if len(content) > 8 << 20:
            raise InvalidMetadata("skill_size_exceeded")
        with zipfile.ZipFile(io.BytesIO(content)) as archive:
            entries = archive.infolist()
            if len(entries) > 1024 or sum(entry.file_size for entry in entries) > 32 << 20:
                raise InvalidMetadata("skill_expansion_exceeded")
            if "SKILL.md" not in archive.namelist():
                raise InvalidMetadata("skill_manifest_missing")
            instructions = archive.read("SKILL.md").decode("utf-8")
            if not instructions.startswith("---\n") or "\n---" not in instructions[4:]:
                raise InvalidMetadata("skill_frontmatter_missing")
            frontmatter = instructions[4:].split("\n---", 1)[0]
            if len(frontmatter) > 16384:
                raise InvalidMetadata("skill_frontmatter_exceeded")
            try:
                header = yaml.safe_load(frontmatter)
            except yaml.YAMLError as error:
                raise InvalidMetadata("invalid_skill_frontmatter") from error
            if (
                not isinstance(header, dict)
                or not isinstance(header.get("name"), str)
                or re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", header["name"]) is None
                or len(header["name"]) > 64
                or not isinstance(header.get("description"), str)
                or not header["description"].strip()
                or len(header["description"]) > 1024
            ):
                raise InvalidMetadata("invalid_skill_frontmatter")
            target = target.parent / header["name"]
            if target.exists():
                raise InvalidMetadata("duplicate_skill_name")
            seen = set()
            for entry in entries:
                name = entry.filename.rstrip("/")
                mode = entry.external_attr >> 16
                if (
                    not name
                    or "\\" in name
                    or name.startswith("/")
                    or any(part in {"", ".", ".."} for part in name.split("/"))
                    or stat.S_IFMT(mode) not in {0, stat.S_IFREG, stat.S_IFDIR}
                    or entry.flag_bits & 1
                    or name in seen
                ):
                    raise InvalidMetadata("unsafe_skill_archive")
                seen.add(name)
                path = target / name
                if entry.is_dir():
                    path.mkdir(parents=True, exist_ok=True)
                else:
                    _write(path, archive.read(entry))
                    if mode & 0o111:
                        path.chmod(0o700)
            if not (target / "SKILL.md").is_file():
                raise InvalidMetadata("skill_manifest_missing")

    @staticmethod
    def verify(target: Path) -> None:
        checksums = json.loads((target / "checksums.json").read_bytes())
        if not isinstance(checksums, dict):
            raise InvalidMetadata("invalid_bundle_manifest")
        entries = list(target.rglob("*"))
        if any(path.is_symlink() for path in entries):
            raise InvalidMetadata("bundle_corrupted")
        files = {str(path.relative_to(target)) for path in entries if path.is_file()}
        if files != set(checksums) | {"checksums.json"}:
            raise InvalidMetadata("bundle_corrupted")
        if hashlib.sha256((target / "metadata.json").read_bytes()).hexdigest() != target.name:
            raise InvalidMetadata("bundle_corrupted")
        for name, checksum in checksums.items():
            if (
                not isinstance(name, str)
                or name.startswith("/")
                or "\\" in name
                or any(part in {"", ".", ".."} for part in name.split("/"))
            ):
                raise InvalidMetadata("invalid_bundle_path")
            path = target / name
            if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != checksum:
                raise InvalidMetadata("bundle_corrupted")

    def record(
        self, snapshot: Snapshot, status: str, *, artifact: Path | None = None, error_code: str = ""
    ) -> None:
        marker = self.marker()
        reports = marker.get("pending_reports", [])
        if not isinstance(reports, list):
            raise InvalidMetadata("invalid_pending_reports")
        result: dict[str, object] = {
            "report_id": str(
                uuid5(
                    NAMESPACE_URL,
                    ":".join(
                        [
                            snapshot.workspace_id,
                            snapshot.agent_config_id,
                            self._replica_id,
                            self._incarnation_id,
                            str(snapshot.selection_revision),
                            snapshot.target_context_version,
                            status,
                            error_code,
                        ]
                    ),
                )
            ),
            "workspace_id": snapshot.workspace_id,
            "agent_config_id": snapshot.agent_config_id,
            "replica_id": self._replica_id,
            "incarnation_id": self._incarnation_id,
            "selection_revision": snapshot.selection_revision,
            "target_context_version": snapshot.target_context_version,
            "status": status,
            "error_code": error_code,
        }
        if status == "applied":
            if artifact is None or snapshot.selection_revision < marker_revision(marker):
                raise InvalidMetadata("stale_application")
            result["applied_context_version"] = snapshot.target_context_version
            marker.update(
                {
                    "selection_revision": snapshot.selection_revision,
                    "target_context_version": snapshot.target_context_version,
                    "artifact_ref": str(artifact.relative_to(self._root)),
                }
            )
        if not any(
            isinstance(r, dict) and r.get("report_id") == result["report_id"] for r in reports
        ):
            reports.append(result)
        marker["pending_reports"] = reports
        self._save(marker)

    def resolve(self, target: str) -> Path | None:
        directory = self._bundles / _segment(target)
        if not directory.exists():
            return None
        candidates = [
            path for path in directory.iterdir() if path.is_dir() and not path.name.startswith(".")
        ]
        if len(candidates) != 1:
            return None
        self.verify(candidates[0])
        return candidates[0]

    def pending(self) -> list[dict[str, object]]:
        values = self.marker().get("pending_reports", [])
        if not isinstance(values, list):
            raise InvalidMetadata("invalid_pending_reports")
        return [value for value in values if isinstance(value, dict)]

    def reported(self, report_id: str) -> None:
        marker = self.marker()
        marker["pending_reports"] = [v for v in self.pending() if v.get("report_id") != report_id]
        self._save(marker)
