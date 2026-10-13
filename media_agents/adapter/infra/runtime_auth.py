"""Verify Gateway request signatures without granting Agents signing authority."""

from __future__ import annotations

import base64
import hashlib
import json
import time
from pathlib import Path
from typing import Protocol

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey


class AssertionScope(Protocol):
    @property
    def workspace_id(self) -> str: ...

    @property
    def agent_config_id(self) -> str: ...

    @property
    def replica_id(self) -> str: ...

    @property
    def incarnation_id(self) -> str: ...


class RuntimeAssertionRejected(ValueError):
    def __init__(self) -> None:
        super().__init__("runtime_assertion_rejected")


def _decode(value: str) -> bytes:
    return base64.b64decode(value + "=" * (-len(value) % 4), altchars=b"-_", validate=True)


class RuntimeAssertionVerifier:
    def __init__(self, keys_file: Path) -> None:
        document = json.loads(keys_file.read_bytes())
        self._keys = {
            key: Ed25519PublicKey.from_public_bytes(_decode(value))
            for key, value in document["keys"].items()
        }
        if not self._keys:
            raise RuntimeAssertionRejected()

    def verify(self, token: str, scope: AssertionScope, method: str, path: str, body: bytes) -> int:
        try:
            if len(token) > 8192:
                raise RuntimeAssertionRejected()
            encoded, signature = token.split(".")
            claims = json.loads(_decode(encoded))
            self._keys[claims["kid"]].verify(_decode(signature), encoded.encode())
            now = int(time.time())
            epoch = claims["execution_epoch"]
            if (
                type(epoch) is not int
                or epoch < 0
                or type(claims["iat"]) is not int
                or type(claims["exp"]) is not int
                or claims["iat"] > now + 5
                or claims["exp"] < now - 5
                or claims["exp"] - claims["iat"] != 60
                or claims["aud"] != "media-agent-runtime"
                or claims["method"] != method
                or claims["path"] != path
                or claims["body_sha256"] != hashlib.sha256(body).hexdigest()
                or tuple(
                    claims[name]
                    for name in ("workspace_id", "agent_config_id", "replica_id", "incarnation_id")
                )
                != (
                    scope.workspace_id,
                    scope.agent_config_id,
                    scope.replica_id,
                    scope.incarnation_id,
                )
            ):
                raise RuntimeAssertionRejected()
            return epoch
        except Exception as error:
            raise RuntimeAssertionRejected() from error
