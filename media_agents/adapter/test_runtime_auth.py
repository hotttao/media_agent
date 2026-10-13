from __future__ import annotations

import base64
import hashlib
import json
import time
from collections.abc import Callable
from pathlib import Path

import pytest
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from fastapi import FastAPI
from fastapi.testclient import TestClient

from adapter.biz.domain.runtime_session.ownership import InstanceScope
from adapter.biz.middleware.runtime_auth import RuntimeAuthMiddleware
from adapter.biz.service.runtime_session.fence import expected_execution_epoch
from adapter.infra.runtime_auth import RuntimeAssertionRejected, RuntimeAssertionVerifier


def encoded(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).decode().rstrip("=")


def authority(tmp_path: Path) -> tuple[InstanceScope, RuntimeAssertionVerifier, Callable[..., str]]:
    key = Ed25519PrivateKey.generate()
    path = tmp_path / "keys.json"
    path.write_text(json.dumps({"keys": {"test": encoded(key.public_key().public_bytes_raw())}}))
    scope = InstanceScope("workspace", "config", "0", "boot")

    def sign(method: str, path: str, body: bytes, epoch: int = 3) -> str:
        now = int(time.time())
        claims = {
            "kid": "test",
            "aud": "media-agent-runtime",
            "iat": now,
            "exp": now + 60,
            "method": method,
            "path": path,
            "body_sha256": hashlib.sha256(body).hexdigest(),
            "workspace_id": scope.workspace_id,
            "agent_config_id": scope.agent_config_id,
            "replica_id": scope.replica_id,
            "incarnation_id": scope.incarnation_id,
            "execution_epoch": epoch,
        }
        message = encoded(json.dumps(claims).encode())
        return message + "." + encoded(key.sign(message.encode()))

    return scope, RuntimeAssertionVerifier(path), sign


def test_signature_binds_request_body_scope_and_time(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    scope, verifier, sign = authority(tmp_path)
    token = sign("POST", "/api/v1/runtime-sessions/s:quiesce", b'{"transfer_id":"t"}')
    assert (
        verifier.verify(
            token, scope, "POST", "/api/v1/runtime-sessions/s:quiesce", b'{"transfer_id":"t"}'
        )
        == 3
    )
    variants = [
        (scope, "GET", "/api/v1/runtime-sessions/s:quiesce", b'{"transfer_id":"t"}'),
        (scope, "POST", "/api/v1/runtime-sessions/other:quiesce", b'{"transfer_id":"t"}'),
        (scope, "POST", "/api/v1/runtime-sessions/s:quiesce", b'{"transfer_id":"other"}'),
        (
            InstanceScope("workspace", "config", "0", "new-boot"),
            "POST",
            "/api/v1/runtime-sessions/s:quiesce",
            b'{"transfer_id":"t"}',
        ),
    ]
    for arguments in variants:
        with pytest.raises(RuntimeAssertionRejected):
            verifier.verify(token, *arguments)
    now = time.time()
    monkeypatch.setattr("adapter.infra.runtime_auth.time.time", lambda: now + 70)
    with pytest.raises(RuntimeAssertionRejected):
        verifier.verify(
            token, scope, "POST", "/api/v1/runtime-sessions/s:quiesce", b'{"transfer_id":"t"}'
        )


def test_managed_api_requires_epoch_and_closes_legacy_execution(tmp_path: Path) -> None:
    scope, verifier, sign = authority(tmp_path)
    app = FastAPI()
    app.state.instance_scope = scope
    app.state.runtime_assertion_verifier = verifier
    app.add_middleware(RuntimeAuthMiddleware)

    @app.post("/api/v1/runtime-sessions/s:quiesce")
    async def write() -> dict[str, int | None]:
        return {"epoch": expected_execution_epoch.get()}

    @app.get("/api/v1/runtime-sessions/s")
    async def read() -> dict[str, int | None]:
        return {"epoch": expected_execution_epoch.get()}

    with TestClient(app) as client:
        path = "/api/v1/runtime-sessions/s:quiesce"
        assert client.post(path).status_code == 401
        assert (
            client.post(
                path, headers={"X-Runtime-Assertion": sign("POST", path, b"", 0)}
            ).status_code
            == 403
        )
        response = client.post(path, headers={"X-Runtime-Assertion": sign("POST", path, b"")})
        assert response.json() == {"epoch": 3}
        assert client.post("/acp").status_code == 403
        path = "/api/v1/runtime-sessions/s"
        assert client.get(
            path, headers={"X-Runtime-Assertion": sign("GET", path, b"", 0)}
        ).json() == {"epoch": None}
    assert expected_execution_epoch.get() is None


def test_go_signed_assertion_is_verified_by_python(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    fixture = json.loads(
        (Path(__file__).parent / "tests/fixtures/runtime_assertion_go.json").read_text()
    )
    keys = tmp_path / "keys.json"
    keys.write_text(json.dumps({"keys": fixture["keys"]}))
    claims = json.loads(base64.urlsafe_b64decode(fixture["token"].split(".")[0] + "=="))
    monkeypatch.setattr("adapter.infra.runtime_auth.time.time", lambda: claims["iat"] + 1)
    verifier = RuntimeAssertionVerifier(keys)
    assert (
        verifier.verify(
            fixture["token"],
            InstanceScope("wire-workspace", "wire-config", "0", "wire-boot"),
            fixture["method"],
            fixture["path"],
            fixture["body"].encode(),
        )
        == 3
    )
