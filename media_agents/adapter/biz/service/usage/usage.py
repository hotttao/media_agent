"""Usage transcript parsing and sweeping shared by agent packages."""

from __future__ import annotations

import json
import logging
import math
import re
import time
from collections.abc import Callable, Iterable, Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Literal

UsageSource = Literal["claude", "codex", "goose"]
UsageSpeed = Literal["standard", "fast"]
FileFingerprint = dict[str, float | int]
SweepCursors = dict[str, FileFingerprint]
WarnFn = Callable[[str], None]

logger = logging.getLogger(__name__)
DEFAULT_SETTLE_GRACE_MS = 10_000
EPOCH_TS = "1970-01-01T00:00:00.000Z"


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
    speed: UsageSpeed | None
    fields_incomplete: bool
    dedup_key: str

    def to_dict(self) -> dict[str, object]:
        return {
            "source": self.source,
            "sessionId": self.session_id,
            "model": self.model,
            "ts": self.ts,
            "inputTokens": self.input_tokens,
            "outputTokens": self.output_tokens,
            "cacheReadTokens": self.cache_read_tokens,
            "cacheCreationTokens": self.cache_creation_tokens,
            "cacheCreation5mTokens": self.cache_creation_5m_tokens,
            "cacheCreation1hTokens": self.cache_creation_1h_tokens,
            "reasoningTokens": self.reasoning_tokens,
            "webSearchRequests": self.web_search_requests,
            "speed": self.speed,
            "fieldsIncomplete": self.fields_incomplete,
            "dedupKey": self.dedup_key,
        }


@dataclass(frozen=True, slots=True)
class UsageSweepResult:
    records: list[UsageRecord]
    cursors: SweepCursors
    has_more: bool

    def to_dict(self) -> dict[str, object]:
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


def parse_claude_transcript(
    lines: Iterable[str],
    *,
    session_id: str | None = None,
    include_trailing: bool = False,
    on_warn: WarnFn | None = None,
) -> list[UsageRecord]:
    """Parse Claude transcript JSONL usage records.

    Claude rewrites assistant entries during streaming, so each message id is deduped to
    the last occurrence. The trailing assistant entry is skipped until the file settles.
    """
    warn = on_warn or _default_warn
    last_by_message_id: dict[str, Mapping[str, object]] = {}
    trailing_message_id: str | None = None

    for index, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError as exc:
            warn(f"claude line {index}: cannot parse json: {exc}")
            continue
        if not isinstance(entry, Mapping):
            trailing_message_id = None
            continue
        if entry.get("type") != "assistant":
            trailing_message_id = None
            continue
        message = _mapping(entry.get("message"))
        usage = _mapping(message.get("usage") if message else None)
        message_id = message.get("id") if message else None
        if not isinstance(message_id, str) or usage is None:
            trailing_message_id = None
            continue
        last_by_message_id[message_id] = entry
        trailing_message_id = message_id

    records: list[UsageRecord] = []
    for message_id, entry in last_by_message_id.items():
        if not include_trailing and message_id == trailing_message_id:
            continue
        message = _mapping(entry.get("message")) or {}
        usage = _mapping(message.get("usage")) or {}
        flags = {"incomplete": False}
        context = f"claude message {message_id}"
        nested_cache = _mapping(usage.get("cache_creation")) or {}
        cache_5m = _num(
            nested_cache.get("ephemeral_5m_input_tokens"),
            context=context,
            field="usage.cache_creation.ephemeral_5m_input_tokens",
            flags=flags,
            warn=warn,
        )
        cache_1h = _num(
            nested_cache.get("ephemeral_1h_input_tokens"),
            context=context,
            field="usage.cache_creation.ephemeral_1h_input_tokens",
            flags=flags,
            warn=warn,
        )
        flat_cache_creation = _num(
            usage.get("cache_creation_input_tokens"),
            context=context,
            field="usage.cache_creation_input_tokens",
            flags=flags,
            warn=warn,
        )
        server_tool_use = _mapping(usage.get("server_tool_use")) or {}
        speed_value = usage.get("service_tier")
        speed: UsageSpeed | None = speed_value if speed_value in ("standard", "fast") else None
        resolved_session_id = session_id or _string(entry.get("sessionId")) or ""
        records.append(
            UsageRecord(
                source="claude",
                session_id=resolved_session_id,
                model=_string(message.get("model")) or "unknown",
                ts=_string(entry.get("timestamp")) or "",
                input_tokens=_num(
                    usage.get("input_tokens"),
                    context=context,
                    field="usage.input_tokens",
                    flags=flags,
                    warn=warn,
                    required=True,
                ),
                output_tokens=_num(
                    usage.get("output_tokens"),
                    context=context,
                    field="usage.output_tokens",
                    flags=flags,
                    warn=warn,
                    required=True,
                ),
                cache_read_tokens=_num(
                    usage.get("cache_read_input_tokens"),
                    context=context,
                    field="usage.cache_read_input_tokens",
                    flags=flags,
                    warn=warn,
                ),
                cache_creation_tokens=max(flat_cache_creation, cache_5m + cache_1h),
                cache_creation_5m_tokens=cache_5m,
                cache_creation_1h_tokens=cache_1h,
                reasoning_tokens=0,
                web_search_requests=_num(
                    server_tool_use.get("web_search_requests"),
                    context=context,
                    field="usage.server_tool_use.web_search_requests",
                    flags=flags,
                    warn=warn,
                ),
                speed=speed,
                fields_incomplete=flags["incomplete"],
                dedup_key=f"claude:{message_id}",
            )
        )
    return records


def parse_codex_rollout(
    lines: Iterable[str],
    *,
    session_id: str = "",
    default_model: str | None = None,
    on_warn: WarnFn | None = None,
) -> list[UsageRecord]:
    """Parse Codex rollout JSONL into per-event token deltas."""
    warn = on_warn or _default_warn
    current_session_id = session_id
    forked_from_id = ""
    model = default_model or "unknown"
    prev = ZERO_TOTALS
    records: list[UsageRecord] = []

    for index, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError as exc:
            warn(f"codex line {index}: cannot parse json: {exc}")
            continue
        if not isinstance(entry, Mapping):
            continue
        payload = _mapping(entry.get("payload")) or {}

        if entry.get("type") == "session_meta":
            current_session_id = (
                _string(payload.get("id"))
                or _string(payload.get("session_id"))
                or current_session_id
            )
            forked_from_id = _string(payload.get("forked_from_id")) or forked_from_id
            model = _string(payload.get("model")) or model
            continue
        if entry.get("type") == "turn_context":
            model = _string(payload.get("model")) or model
            continue
        if entry.get("type") != "event_msg" or payload.get("type") != "token_count":
            continue

        info = _mapping(payload.get("info"))
        if info is None:
            continue
        model = _string(info.get("model")) or model
        total_usage = _mapping(info.get("total_token_usage"))
        if total_usage is None:
            continue

        flags = {"incomplete": False}
        context = f"codex token_count line {index}"
        input_raw = _num(
            total_usage.get("input_tokens"),
            context=context,
            field="total_token_usage.input_tokens",
            flags=flags,
            warn=warn,
        )
        cached = _num(
            total_usage.get("cached_input_tokens"),
            context=context,
            field="total_token_usage.cached_input_tokens",
            flags=flags,
            warn=warn,
        )
        cur = _CodexTotals(
            uncached_input=max(0, input_raw - cached),
            cache_read=cached,
            output=_num(
                total_usage.get("output_tokens"),
                context=context,
                field="total_token_usage.output_tokens",
                flags=flags,
                warn=warn,
            ),
            reasoning=_num(
                total_usage.get("reasoning_output_tokens"),
                context=context,
                field="total_token_usage.reasoning_output_tokens",
                flags=flags,
                warn=warn,
            ),
            total=_num(
                total_usage.get("total_tokens"),
                context=context,
                field="total_token_usage.total_tokens",
                flags=flags,
                warn=warn,
                required=True,
            ),
        )
        base = ZERO_TOTALS if cur.total < prev.total else prev
        record = UsageRecord(
            source="codex",
            session_id=current_session_id,
            model=model,
            ts=_string(entry.get("timestamp")) or "",
            input_tokens=max(0, cur.uncached_input - base.uncached_input),
            output_tokens=max(0, cur.output - base.output),
            cache_read_tokens=max(0, cur.cache_read - base.cache_read),
            cache_creation_tokens=0,
            cache_creation_5m_tokens=0,
            cache_creation_1h_tokens=0,
            reasoning_tokens=max(0, cur.reasoning - base.reasoning),
            web_search_requests=0,
            speed=None,
            fields_incomplete=flags["incomplete"],
            dedup_key=f"codex:{forked_from_id or current_session_id}:{cur.total}",
        )
        prev = cur
        if _is_noop(record):
            continue
        records.append(record)
    return records


def parse_acp_usage_log(
    lines: Iterable[str],
    *,
    session_id: str = "",
    default_model: str | None = None,
    on_warn: WarnFn | None = None,
) -> list[UsageRecord]:
    """Parse Goose ACP usage logs from ``$HOME/.acp-usage``."""
    warn = on_warn or _default_warn
    records: list[UsageRecord] = []
    prev_input = 0
    prev_output = 0

    for index, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(entry, Mapping):
            continue

        model = _string(entry.get("model")) or default_model or "unknown"
        ts = _string(entry.get("ts")) or _string(entry.get("timestamp"))
        if not ts:
            warn(f"goose line {index}: missing timestamp, using epoch")
            ts = EPOCH_TS

        accumulated_input = _raw_num(entry.get("accumulated_input_tokens"))
        accumulated_output = _raw_num(entry.get("accumulated_output_tokens"))
        if accumulated_input is not None or accumulated_output is not None:
            current_input = accumulated_input or 0
            current_output = accumulated_output or 0
            input_tokens = max(0, current_input - prev_input)
            output_tokens = max(0, current_output - prev_output)
            prev_input = current_input
            prev_output = current_output
            dedup_suffix = str(current_input + current_output)
        else:
            input_tokens = _raw_num(entry.get("input_tokens")) or 0
            output_tokens = _raw_num(entry.get("output_tokens")) or 0
            dedup_suffix = str(index)

        record = UsageRecord(
            source="goose",
            session_id=session_id,
            model=model,
            ts=ts,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            cache_read_tokens=0,
            cache_creation_tokens=0,
            cache_creation_5m_tokens=0,
            cache_creation_1h_tokens=0,
            reasoning_tokens=0,
            web_search_requests=0,
            speed=None,
            fields_incomplete=True,
            dedup_key=f"goose:{session_id}:{dedup_suffix}",
        )
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
    settle_grace_ms: int = DEFAULT_SETTLE_GRACE_MS,
    now_ms: int | None = None,
    on_warn: WarnFn | None = None,
) -> UsageSweepResult:
    """Sweep changed Claude, Codex, and Goose usage files under an agent home."""
    home = Path(home_dir)
    previous = cursors or {}
    next_cursors: SweepCursors = {}
    records: list[UsageRecord] = []
    processed = 0
    has_more = False
    now = now_ms if now_ms is not None else int(time.time() * 1000)

    def claim_changed(file: Path) -> FileFingerprint | None:
        nonlocal has_more, processed
        fp = _fingerprint(file)
        if fp is None:
            return None
        key = str(file)
        if _fingerprints_equal(previous.get(key), fp):
            next_cursors[key] = fp
            return None
        if max_files is not None and processed >= max_files:
            has_more = True
            return None
        next_cursors[key] = fp
        processed += 1
        return fp

    for file, parsed_session_id in _list_claude_files(home / ".claude" / "projects"):
        fp = claim_changed(file)
        if fp is None:
            continue
        include_trailing = now - int(fp["mtimeMs"]) > settle_grace_ms
        records.extend(
            parse_claude_transcript(
                _read_lines(file),
                session_id=parsed_session_id,
                include_trailing=include_trailing,
                on_warn=on_warn,
            )
        )
        if not include_trailing:
            next_cursors.pop(str(file), None)

    for file in _list_codex_rollouts(home / ".codex" / "sessions"):
        if claim_changed(file) is None:
            continue
        records.extend(
            parse_codex_rollout(
                _read_lines(file),
                session_id=_fallback_codex_session_id(file),
                default_model=fallback_model,
                on_warn=on_warn,
            )
        )

    for file in _list_jsonl_files(home / ".acp-usage"):
        if claim_changed(file) is None:
            continue
        records.extend(
            parse_acp_usage_log(
                _read_lines(file),
                session_id=file.stem,
                default_model=fallback_model,
                on_warn=on_warn,
            )
        )

    return UsageSweepResult(records=records, cursors=next_cursors, has_more=has_more)


def _list_claude_files(projects_dir: Path) -> list[tuple[Path, str]]:
    files: list[tuple[Path, str]] = []
    for slug_dir in _iter_dirs(projects_dir):
        for file in _list_jsonl_files(slug_dir):
            session_id = file.stem
            files.append((file, session_id))
            for subagent_file in _list_subagent_files(slug_dir / session_id / "subagents"):
                files.append((subagent_file, session_id))
    return sorted(files, key=lambda item: str(item[0]))


def _list_subagent_files(subagents_dir: Path) -> list[Path]:
    return sorted(
        file
        for file in _safe_iterdir(subagents_dir)
        if file.is_file() and file.name.startswith("agent-") and file.suffix == ".jsonl"
    )


def _list_codex_rollouts(sessions_dir: Path) -> list[Path]:
    if not sessions_dir.exists():
        return []
    try:
        return sorted(path for path in sessions_dir.rglob("rollout-*.jsonl") if path.is_file())
    except OSError:
        return []


def _list_jsonl_files(directory: Path) -> list[Path]:
    return sorted(
        file for file in _safe_iterdir(directory) if file.is_file() and file.suffix == ".jsonl"
    )


def _iter_dirs(directory: Path) -> list[Path]:
    return sorted(path for path in _safe_iterdir(directory) if path.is_dir())


def _safe_iterdir(directory: Path) -> list[Path]:
    try:
        return list(directory.iterdir())
    except OSError:
        return []


def _read_lines(path: Path) -> list[str]:
    try:
        return path.read_text(encoding="utf-8").splitlines()
    except OSError:
        return []


def _fallback_codex_session_id(path: Path) -> str:
    return re.sub(r"^rollout-[0-9T-]+-", "", path.stem)


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


def _fingerprints_equal(left: FileFingerprint | None, right: FileFingerprint) -> bool:
    if left is None:
        return False
    return (
        left.get("dev") == right.get("dev")
        and left.get("ino") == right.get("ino")
        and left.get("size") == right.get("size")
        and left.get("mtimeMs") == right.get("mtimeMs")
    )


def _mapping(value: object) -> Mapping[str, object] | None:
    return value if isinstance(value, Mapping) else None


def _string(value: object) -> str | None:
    return value if isinstance(value, str) else None


def _raw_num(value: object) -> int | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, int | float) and math.isfinite(float(value)):
        return int(value)
    return None


def _num(
    value: object,
    *,
    context: str,
    field: str,
    flags: dict[str, bool],
    warn: WarnFn,
    required: bool = False,
) -> int:
    parsed = _raw_num(value)
    if parsed is not None:
        return parsed
    if value is None and not required:
        return 0
    flags["incomplete"] = True
    reason = "missing" if value is None else f"invalid {value!r}"
    warn(f"{context}: {reason} {field}, defaulting to 0")
    return 0


def _is_noop(record: UsageRecord) -> bool:
    return (
        record.input_tokens == 0
        and record.output_tokens == 0
        and record.cache_read_tokens == 0
        and record.cache_creation_tokens == 0
        and record.reasoning_tokens == 0
        and record.web_search_requests == 0
    )


def _default_warn(message: str) -> None:
    logger.warning("[agent-usage] %s", message)
