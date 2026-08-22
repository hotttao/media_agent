from __future__ import annotations

import json
from pathlib import Path

from adapter.biz.service.usage.usage import (
    parse_acp_usage_log,
    parse_claude_transcript,
    parse_codex_rollout,
    sweep_usage,
)


def _session_meta(session_id: str, **payload: object) -> str:
    return json.dumps({"type": "session_meta", "payload": {"id": session_id, **payload}})


def _token_count(total: dict[str, object], ts: str = "t") -> str:
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


def _claude_assistant(message_id: str, input_tokens: object, output_tokens: object) -> str:
    return json.dumps(
        {
            "type": "assistant",
            "sessionId": "claude-session",
            "timestamp": "t",
            "message": {
                "id": message_id,
                "model": "claude-test",
                "usage": {
                    "input_tokens": input_tokens,
                    "output_tokens": output_tokens,
                    "cache_read_input_tokens": 3,
                    "cache_creation_input_tokens": 2,
                    "cache_creation": {
                        "ephemeral_5m_input_tokens": 4,
                        "ephemeral_1h_input_tokens": 5,
                    },
                    "server_tool_use": {"web_search_requests": 1},
                    "service_tier": "fast",
                },
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


def test_parse_codex_rollout_marks_invalid_required_totals_incomplete() -> None:
    warnings: list[str] = []
    records = parse_codex_rollout(
        [_session_meta("s1"), _token_count({"input_tokens": 5, "total_tokens": "bad"})],
        on_warn=warnings.append,
    )

    assert len(records) == 1
    assert records[0].fields_incomplete is True
    assert warnings


def test_parse_claude_transcript_dedupes_and_skips_trailing_assistant() -> None:
    records = parse_claude_transcript(
        [
            _claude_assistant("m1", 10, 2),
            _claude_assistant("m1", 12, 3),
            _claude_assistant("m2", 20, 4),
        ],
        include_trailing=False,
    )

    assert len(records) == 1
    assert records[0].dedup_key == "claude:m1"
    assert records[0].input_tokens == 12
    assert records[0].output_tokens == 3
    assert records[0].cache_creation_tokens == 9
    assert records[0].speed == "fast"
    assert records[0].web_search_requests == 1


def test_parse_claude_transcript_can_include_trailing_assistant() -> None:
    records = parse_claude_transcript([_claude_assistant("m1", 10, 2)], include_trailing=True)

    assert len(records) == 1
    assert records[0].source == "claude"
    assert records[0].session_id == "claude-session"


def test_parse_acp_usage_log_supports_accumulated_and_direct_shapes() -> None:
    accumulated = parse_acp_usage_log(
        [
            json.dumps(
                {
                    "ts": "t1",
                    "model": "goose-test",
                    "accumulated_input_tokens": 5,
                    "accumulated_output_tokens": 2,
                }
            ),
            json.dumps(
                {
                    "ts": "t2",
                    "model": "goose-test",
                    "accumulated_input_tokens": 8,
                    "accumulated_output_tokens": 7,
                }
            ),
        ],
        session_id="g1",
    )
    direct = parse_acp_usage_log(
        [json.dumps({"timestamp": "t", "input_tokens": 4, "output_tokens": 1})],
        session_id="g2",
        default_model="goose-default",
    )

    assert [(record.input_tokens, record.output_tokens) for record in accumulated] == [
        (5, 2),
        (3, 5),
    ]
    assert accumulated[0].dedup_key == "goose:g1:7"
    assert direct[0].dedup_key == "goose:g2:0"
    assert direct[0].model == "goose-default"
    assert direct[0].fields_incomplete is True


def test_sweep_usage_scans_claude_codex_and_goose(tmp_path: Path) -> None:
    claude = tmp_path / ".claude" / "projects" / "workspace" / "claude-session.jsonl"
    claude.parent.mkdir(parents=True)
    claude.write_text(_claude_assistant("m1", 10, 2), encoding="utf-8")

    codex = (
        tmp_path
        / ".codex"
        / "sessions"
        / "2026"
        / "07"
        / "24"
        / "rollout-2026-07-24T00-00-00-s1.jsonl"
    )
    codex.parent.mkdir(parents=True)
    codex.write_text(
        "\n".join(
            [
                _session_meta("s1"),
                _token_count({"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}),
            ]
        ),
        encoding="utf-8",
    )

    goose = tmp_path / ".acp-usage" / "g1.jsonl"
    goose.parent.mkdir(parents=True)
    goose.write_text(
        json.dumps({"ts": "t", "input_tokens": 4, "output_tokens": 1}),
        encoding="utf-8",
    )

    result = sweep_usage(home_dir=tmp_path, fallback_model="gpt-test", settle_grace_ms=0)
    second = sweep_usage(
        home_dir=tmp_path,
        cursors=result.cursors,
        fallback_model="gpt-test",
        settle_grace_ms=0,
    )

    assert [record.source for record in result.records] == ["claude", "codex", "goose"]
    assert result.records[0].session_id == "claude-session"
    assert result.records[1].session_id == "s1"
    assert result.records[2].session_id == "g1"
    assert second.records == []
    assert second.cursors == result.cursors


def test_sweep_usage_re_reads_unsettled_claude_tail(tmp_path: Path) -> None:
    claude = tmp_path / ".claude" / "projects" / "workspace" / "claude-session.jsonl"
    claude.parent.mkdir(parents=True)
    claude.write_text(_claude_assistant("m1", 10, 2), encoding="utf-8")

    result = sweep_usage(home_dir=tmp_path, settle_grace_ms=10_000, now_ms=0)

    assert result.records == []
    assert str(claude) not in result.cursors
