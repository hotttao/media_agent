from __future__ import annotations

import json
from pathlib import Path

from fastapi import FastAPI
from fastapi.testclient import TestClient

from adapter.biz.api.v1.routes.usage.router import build_usage_router


def test_usage_route_returns_records_and_cursors(tmp_path: Path) -> None:
    rollout = tmp_path / ".codex" / "sessions" / "2026" / "07" / "24" / "rollout-x-s1.jsonl"
    rollout.parent.mkdir(parents=True)
    rollout.write_text(
        "\n".join(
            [
                json.dumps({"type": "session_meta", "payload": {"id": "s1"}}),
                json.dumps(
                    {
                        "type": "event_msg",
                        "timestamp": "t",
                        "payload": {
                            "type": "token_count",
                            "info": {
                                "total_token_usage": {
                                    "input_tokens": 10,
                                    "output_tokens": 2,
                                    "total_tokens": 12,
                                }
                            },
                        },
                    }
                ),
            ]
        ),
        encoding="utf-8",
    )
    app = FastAPI()
    app.include_router(build_usage_router(home_dir=tmp_path, fallback_model=lambda: "gpt-test"))

    response = TestClient(app).post("/usage", json={})

    assert response.status_code == 200
    body = response.json()
    assert body["hasMore"] is False
    assert len(body["records"]) == 1
    assert body["records"][0]["source"] == "codex"
    assert body["records"][0]["sessionId"] == "s1"
    assert body["records"][0]["inputTokens"] == 10
    assert body["records"][0]["model"] == "gpt-test"
    assert body["cursors"]


def test_usage_route_accepts_typed_cursors_and_max_files(tmp_path: Path) -> None:
    first = tmp_path / ".acp-usage" / "g1.jsonl"
    second = tmp_path / ".acp-usage" / "g2.jsonl"
    first.parent.mkdir(parents=True)
    first.write_text(
        json.dumps({"ts": "t", "input_tokens": 4, "output_tokens": 1}),
        encoding="utf-8",
    )
    second.write_text(
        json.dumps({"ts": "t", "input_tokens": 5, "output_tokens": 2}),
        encoding="utf-8",
    )
    app = FastAPI()
    app.include_router(build_usage_router(home_dir=tmp_path))

    response = TestClient(app).post("/usage", json={"maxFiles": 1, "cursors": {}})

    assert response.status_code == 200
    body = response.json()
    assert body["hasMore"] is True
    assert len(body["records"]) == 1