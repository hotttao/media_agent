from __future__ import annotations

import json
from pathlib import Path

from adapter.biz.service.usage.usage import parse_codex_rollout, sweep_usage


def _session_meta(session_id: str, **payload: object) -> str:
    return json.dumps({"type": "session_meta", "payload": {"id": session_id, **payload}})


def _token_count(total: dict[str, int], ts: str = "t") -> str:
    return json.dumps(
        {
            "type": "event_msg",
            "timestamp": ts,
            "payload": {
                "type": "token_count",
                "info": {"total_token_usage": total},
            },
        }
    )


def test_parse_codex_rollout_emits_cumulative_deltas() -> None:
    records = parse_codex_rollout(
        [
            _session_meta("s1"),
            _token_count({"input_tokens": 100, "cached_input_tokens": 20, "total_tokens": 100}),
            _token_count({"input_tokens": 250, "cached_input_tokens": 50, "total_tokens": 250}),
        ],
        default_model="gpt-test",
    )

    assert [record.input_tokens for record in records] == [80, 120]
    assert [record.cache_read_tokens for record in records] == [20, 30]
    assert [record.dedup_key for record in records] == ["codex:s1:100", "codex:s1:250"]
    assert records[0].model == "gpt-test"


def test_sweep_usage_skips_unchanged_files(tmp_path: Path) -> None:
    rollout = tmp_path / ".codex" / "sessions" / "2026" / "07" / "24" / "rollout-x-s1.jsonl"
    rollout.parent.mkdir(parents=True)
    rollout.write_text(
        "\n".join(
            [
                _session_meta("s1"),
                _token_count({"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}),
            ]
        ),
        encoding="utf-8",
    )

    first = sweep_usage(home_dir=tmp_path, fallback_model="gpt-test")
    second = sweep_usage(home_dir=tmp_path, cursors=first.cursors, fallback_model="gpt-test")

    assert len(first.records) == 1
    assert first.records[0].input_tokens == 10
    assert second.records == []
    assert second.cursors == first.cursors
