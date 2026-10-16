from __future__ import annotations

import base64
import re

import httpx

from adapter.biz.domain.runtime_metadata.entity import InvalidMetadata, Resource, Snapshot


def _text(value: object) -> str:
    if not isinstance(value, str):
        raise InvalidMetadata("invalid_string")
    return value


def _positive(value: object) -> int:
    if type(value) is not int or value <= 0:
        raise InvalidMetadata("invalid_revision")
    return value


def _decode_snapshot_v1(value: object) -> Snapshot:
    if (
        not isinstance(value, dict)
        or type(value.get("metadata_schema_version")) is not int
        or value.get("metadata_schema_version") != 1
    ):
        raise InvalidMetadata("incompatible_schema")
    if value.get("status", "current") not in {"current", "superseded"}:
        raise InvalidMetadata("unknown_metadata_status")
    if not isinstance(value.get("resources", []), list):
        raise InvalidMetadata("invalid_resources")
    resources = []
    for raw in value.get("resources", []):
        if not isinstance(raw, dict):
            raise InvalidMetadata("invalid_resource")
        kind = _text(raw.get("kind"))
        if kind not in {
            "media_control.skill",
            "media_control.configured_prompt",
            "media_control.memory_store",
            "media_plugin.mcp_binding",
        }:
            raise InvalidMetadata("unknown_required_resource")
        checksum = _text(raw.get("checksum"))
        if not re.fullmatch(r"[a-f0-9]{64}", checksum):
            raise InvalidMetadata("invalid_checksum")
        resources.append(
            Resource(
                kind,
                _text(raw.get("id")),
                _positive(raw.get("version")),
                _text(raw.get("name", "")),
                checksum,
                _text(raw.get("content_ref", "")),
                _text(raw.get("content", "")),
            )
        )
    return Snapshot(
        _text(value.get("workspace_id")),
        _text(value.get("agent_config_id")),
        _positive(value.get("selection_revision")),
        _text(value.get("target_context_version")),
        _positive(value.get("context_version")),
        _text(value.get("agent_type")),
        _text(value.get("configured_prompt", "")),
        _text(value.get("platform_prompt", "")),
        _text(value.get("platform_prompt_template_version")),
        tuple(resources),
        _text(value.get("status", "current")),
    )


def decode_snapshot(value: object) -> Snapshot:
    try:
        return _decode_snapshot_v1(value)
    except InvalidMetadata as error:
        if isinstance(value, dict):
            try:
                context = Snapshot(
                    _text(value.get("workspace_id")),
                    _text(value.get("agent_config_id")),
                    _positive(value.get("selection_revision")),
                    _text(value.get("target_context_version")),
                    _positive(value.get("context_version")),
                    _text(value.get("agent_type")),
                    "",
                    "",
                    "unsupported",
                    (),
                )
            except InvalidMetadata:
                raise error
            raise InvalidMetadata("metadata_schema_incompatible", context) from error
        raise


class GatewayMetadataClient:
    def __init__(
        self,
        base_url: str,
        token: str,
        workspace_id: str,
        config_id: str,
        replica_id: str,
        incarnation_id: str,
        instance_credential: str,
    ) -> None:
        self._http = httpx.AsyncClient(
            base_url=base_url.rstrip("/"),
            headers={
                "Authorization": f"Bearer {token}",
                "X-Agent-Instance-Credential": instance_credential,
            },
            timeout=10,
            follow_redirects=False,
        )
        self._scope = {
            "workspace_id": workspace_id,
            "agent_config_id": config_id,
            "replica_id": replica_id,
            "incarnation_id": incarnation_id,
        }

    async def fetch(
        self, *, selection_revision: int = 0, target: str = "", for_session: bool = False
    ) -> Snapshot | None:
        params = {
            **self._scope,
            "target_context_version": target,
            "selection_revision": str(selection_revision),
            "for_session": str(for_session).lower(),
        }
        response = await self._http.get("/api/agent/v1/runtime-context", params=params)
        if response.status_code == 404:
            return None
        response.raise_for_status()
        value = response.json()
        if (
            not isinstance(value, dict)
            or value.get("workspace_id") != self._scope["workspace_id"]
            or value.get("agent_config_id") != self._scope["agent_config_id"]
        ):
            raise InvalidMetadata("scope_mismatch")
        snapshot = decode_snapshot(value)
        if (
            snapshot.workspace_id != self._scope["workspace_id"]
            or snapshot.agent_config_id != self._scope["agent_config_id"]
        ):
            raise InvalidMetadata("scope_mismatch")
        return snapshot

    async def artifact(self, target: str, checksum: str) -> bytes:
        response = await self._http.get(
            f"/api/agent/v1/metadata-artifacts/{checksum}",
            params={**self._scope, "target_context_version": target},
        )
        response.raise_for_status()
        value = response.json()
        if value.get("checksum") != checksum:
            raise InvalidMetadata("artifact_checksum_mismatch")
        return base64.b64decode(value["content"], validate=True)

    async def report(self, result: dict[str, object]) -> None:
        response = await self._http.post("/api/agent/v1/metadata-results", json=result)
        response.raise_for_status()
        if response.json().get("accepted") is not True:
            raise InvalidMetadata("report_unaccepted")

    async def close(self) -> None:
        await self._http.aclose()
