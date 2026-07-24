"""Usage transcript parsing and sweeping shared by agent packages."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Literal

UsageSource = Literal["codex"]
FileFingerprint = dict[str, float | int]
SweepCursors = dict[str, FileFingerprint]


@dataclass(frozen=True, slots=True)
class UsageRecord:
    source: UsageSource
    session_id: str
    model: str
    ts: str
    input_tokens: int
    output_tokens: int
    cache_read_tokens: int
    cache_creation_tokens: int
    cache_creation_5m_tokens: int
    cache_creation_1h_tokens: int
    reasoning_tokens: int
    web_search_requests: int
    speed: str | None
    fields_incomplete: bool
    dedup_key: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
class UsageSweepResult:
    records: list[UsageRecord]
    cursors: SweepCursors
    has_more: bool

    def to_dict(self) -> dict[str, Any]:
        return {
            "records": [record.to_dict() for record in self.records],
            "cursors": self.cursors,
            "hasMore": self.has_more,
        }


@dataclass(frozen=True, slots=True)
class _CodexTotals:
    uncached_input: int = 0
    cache_read: int = 0
    output: int = 0
    reasoning: int = 0
    total: int = 0


ZERO_TOTALS = _CodexTotals()


def parse_codex_rollout(
    lines: list[str],
    *,
    session_id: str = "",
    default_model: str | None = None,
) -> list[UsageRecord]:
    """Parse Codex rollout JSONL into per-event token deltas."""
    current_session_id = session_id
    forked_from_id = ""
    model = default_model or "unknown"
    prev = ZERO_TOTALS
    records: list[UsageRecord] = []

    for line in lines:
        if not line.strip():
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(entry, dict):
            continue
        payload = entry.get("payload") if isinstance(entry.get("payload"), dict) else {}

        if entry.get("type") == "session_meta":
            current_session_id = str(
                payload.get("id") or payload.get("session_id") or current_session_id
            )
            forked_from_id = str(payload.get("forked_from_id") or forked_from_id)
            if isinstance(payload.get("model"), str):
                model = payload["model"]
            continue
        if entry.get("type") == "turn_context":
            if isinstance(payload.get("model"), str):
                model = payload["model"]
            continue
        if entry.get("type") != "event_msg" or payload.get("type") != "token_count":
            continue

        info = payload.get("info")
        if not isinstance(info, dict):
            continue
        if isinstance(info.get("model"), str):
            model = info["model"]
        total_usage = info.get("total_token_usage")
        if not isinstance(total_usage, dict):
            continue

        input_raw = _num(total_usage.get("input_tokens"))
        cached = _num(total_usage.get("cached_input_tokens"))
        cur = _CodexTotals(
            uncached_input=max(0, input_raw - cached),
            cache_read=cached,
            output=_num(total_usage.get("output_tokens")),
            reasoning=_num(total_usage.get("reasoning_output_tokens")),
            total=_num(total_usage.get("total_tokens")),
        )
        base = ZERO_TOTALS if cur.total < prev.total else prev
        record = UsageRecord(
            source="codex",
            session_id=current_session_id,
            model=model,
            ts=str(entry.get("timestamp") or ""),
            input_tokens=max(0, cur.uncached_input - base.uncached_input),
            output_tokens=max(0, cur.output - base.output),
            cache_read_tokens=max(0, cur.cache_read - base.cache_read),
            cache_creation_tokens=0,
            cache_creation_5m_tokens=0,
            cache_creation_1h_tokens=0,
            reasoning_tokens=max(0, cur.reasoning - base.reasoning),
            web_search_requests=0,
            speed=None,
            fields_incomplete=False,
            dedup_key=f"codex:{forked_from_id or current_session_id}:{cur.total}",
        )
        prev = cur
        if _is_noop(record):
            continue
        records.append(record)
    return records


def sweep_usage(
    *,
    home_dir: str | Path,
    cursors: SweepCursors | None = None,
    max_files: int | None = None,
    fallback_model: str | None = None,
) -> UsageSweepResult:
    """Sweep changed Codex rollout files under ``$HOME/.codex/sessions``."""
    previous = cursors or {}
    next_cursors: SweepCursors = {}
    records: list[UsageRecord] = []
    processed = 0
    has_more = False

    for file in _list_codex_rollouts(Path(home_dir) / ".codex" / "sessions"):
        fp = _fingerprint(file)
        if fp is None:
            continue
        key = str(file)
        if previous.get(key) == fp:
            next_cursors[key] = fp
            continue
        if max_files is not None and processed >= max_files:
            has_more = True
            continue
        next_cursors[key] = fp
        processed += 1
        fallback_id = file.stem.split("-", maxsplit=2)[-1]
        records.extend(
            parse_codex_rollout(
                file.read_text(encoding="utf-8").splitlines(),
                session_id=fallback_id,
                default_model=fallback_model,
            )
        )

    return UsageSweepResult(records=records, cursors=next_cursors, has_more=has_more)


def _list_codex_rollouts(sessions_dir: Path) -> list[Path]:
    if not sessions_dir.exists():
        return []
    return sorted(path for path in sessions_dir.rglob("rollout-*.jsonl") if path.is_file())


def _fingerprint(path: Path) -> FileFingerprint | None:
    try:
        stat = path.stat()
    except OSError:
        return None
    return {
        "dev": stat.st_dev,
        "ino": stat.st_ino,
        "size": stat.st_size,
        "mtimeMs": stat.st_mtime_ns // 1_000_000,
    }


def _num(value: Any) -> int:
    if isinstance(value, bool):
        return 0
    if isinstance(value, int | float) and value == value:
        return int(value)
    return 0


def _is_noop(record: UsageRecord) -> bool:
    return (
        record.input_tokens == 0
        and record.output_tokens == 0
        and record.cache_read_tokens == 0
        and record.reasoning_tokens == 0
    )
