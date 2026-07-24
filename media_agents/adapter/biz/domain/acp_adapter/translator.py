"""ACP SessionUpdate -> UniversalEvent 翻译器。

对标 internal/acp-adapter/universal-events.ts 的 AcpEventTranslator。
有状态：一个 turn 一个实例，跨 SessionUpdate 维护 currentMessageItem /
activeToolCalls / lastUsageUpdate，产出 UniversalEvent 生命周期事件。

数据流：
    SDK session_update 通知
      -> AcpSession.session_update
      -> translator.translate(update) -> list[UniversalEvent]
      -> SessionSink.write  (json.dumps)

生命周期产出：
    session.started / session.ended / error
    item.started / item.delta / item.completed
    question.requested

骨架只实现核心 union 翻译（message / tool_call / tool_call_update / usage）。
agent-specific 补偿（goose _meta.goose.toolCall.toolName 提取、codex 图片生成
status 修正、mcp_startup 伪 tool_call 抑制等）由 agent 包子类 override
对应的 _on_* 钩子实现，不在 domain 层硬编码。
"""

from __future__ import annotations

import itertools
import json
import logging
import time
from typing import TYPE_CHECKING, Any

from acp.schema import (
    AgentMessageChunk,
    AgentPlanContentUpdate,
    AgentPlanRemovedUpdate,
    AgentPlanUpdate,
    AgentThoughtChunk,
    AvailableCommandsUpdate,
    ConfigOptionUpdate,
    CurrentModeUpdate,
    PromptResponse,
    SessionInfoUpdate,
    TextContentBlock,
    ToolCallProgress,
    ToolCallStart,
    UsageUpdate,
    UserMessageChunk,
)

from adapter.biz.domain.acp_adapter.entity import PromptResult
from adapter.biz.domain.acp_adapter.llm_event import (
    ContentDelta,
    ContentPart,
    SessionEndReason,
    TurnStats,
    UniversalEvent,
    UniversalItem,
)

if TYPE_CHECKING:
    from collections.abc import Iterator

logger = logging.getLogger(__name__)


# ── 模块级时间戳 / item_id 生成 ──────────────────────────

_counter: Iterator[int] = itertools.count(1)


def _ts() -> int:
    """当前毫秒时间戳。对标 TS ts() = Date.now()。"""
    return int(time.time() * 1000)


def _next_item_id() -> str:
    """生成全局唯一 item_id。对标 TS nextItemId()。"""
    return f"item_{_ts()}_{next(_counter)}"


# ── active tool call 跟踪 ────────────────────────────────


class _ActiveToolCall:
    """一个正在进行的 tool_call 的跟踪状态。

    对标 TS activeToolCalls Map 的 value。ToolCallUpdate 的 status / rawInput
    等字段按 ACP spec 是可选的（omitted = unchanged），必须从跟踪状态继承
    而不是默认 "completed"，否则会对一个 call emit N 个 terminal 事件。
    """

    __slots__ = ("item_id", "title", "raw_input", "last_status", "terminal_emitted")

    def __init__(self, item_id: str, title: str, raw_input: Any, last_status: str) -> None:
        self.item_id = item_id
        self.title = title
        self.raw_input = raw_input
        self.last_status = last_status
        self.terminal_emitted: bool = False


# ── translator ───────────────────────────────────────────


class AcpEventTranslator:
    """有状态翻译器：一个 turn 一个实例。

    对标 TS AcpEventTranslator。跨 SessionUpdate 维护：
      - current_message_item: 当前 assistant 消息项（跨 text delta 累积）
      - accumulated_text: 当前消息累积的文本（finalize 时回填 content）
      - active_tool_calls: tool_call_id -> _ActiveToolCall
      - last_usage_update: 最后一次 usage_update（buildStats 用）
      - turn_start_ms: turn 开始时间（buildStats 算 duration 用）
    """

    def __init__(self, session_id: str | None = None) -> None:
        self.session_id: str | None = session_id
        self.current_message_item: UniversalItem | None = None
        self.accumulated_text: str = ""
        self.active_tool_calls: dict[str, _ActiveToolCall] = {}
        self.last_usage_update: UsageUpdate | None = None
        self.turn_start_ms: int = 0

    def set_session_id(self, session_id: str) -> None:
        self.session_id = session_id

    # ── session 生命周期 ──────────────────────────────────

    def session_started(self, session_id: str) -> UniversalEvent:
        self.session_id = session_id
        return UniversalEvent(type="session.started", timestamp=_ts(), session_id=session_id)

    def session_ended(
        self, reason: SessionEndReason, stats: TurnStats | None = None
    ) -> UniversalEvent:
        return UniversalEvent(
            type="session.ended",
            timestamp=_ts(),
            session_id=self.session_id,
            reason=reason,
            stats=stats,
        )

    def error(self, message: str, code: str | None = None) -> UniversalEvent:
        return UniversalEvent(
            type="error",
            timestamp=_ts(),
            session_id=self.session_id,
            message=message,
            code=code,
        )

    # ── SessionUpdate -> UniversalEvent[] ─────────────────

    def translate(self, update: Any) -> list[UniversalEvent]:
        """翻译单个 SDK SessionUpdate 为 0 或多个 UniversalEvent。

        用 isinstance 判断 SDK union 的具体子类型（对标 TS switch on
        update.sessionUpdate），类型安全且字段访问经过 mypy 推断。
        """
        if not self.turn_start_ms:
            self.turn_start_ms = _ts()

        # 按出现频率排序， minimizing isinstance 检查次数
        if isinstance(update, AgentMessageChunk):
            return self._on_agent_message(update)
        if isinstance(update, ToolCallProgress):
            return self._on_tool_call_update(update)
        if isinstance(update, ToolCallStart):
            return self._on_tool_call(update)
        if isinstance(update, UsageUpdate):
            return self._on_usage(update)
        if isinstance(update, AgentThoughtChunk):
            return self._on_thought(update)
        if isinstance(update, UserMessageChunk):
            return self._on_user_message(update)
        if isinstance(update, (AgentPlanUpdate, AgentPlanContentUpdate, AgentPlanRemovedUpdate)):
            return self._on_plan(update)
        if isinstance(
            update,
            (
                AvailableCommandsUpdate,
                CurrentModeUpdate,
                SessionInfoUpdate,
                ConfigOptionUpdate,
            ),
        ):
            return self._on_meta_update(update)
        logger.debug("unhandled session_update: %r", type(update).__name__)
        return []

    # ── agent_message_chunk -> item.started + item.delta ──

    def _on_agent_message(self, update: AgentMessageChunk) -> list[UniversalEvent]:
        block = update.content
        if not isinstance(block, TextContentBlock):
            logger.debug(
                "agent_message_chunk dropped: non-text content=%r session=%s",
                type(block).__name__,
                self.session_id,
            )
            return []

        events: list[UniversalEvent] = []
        # 首个 chunk：创建 current_message_item 并 emit item.started
        if self.current_message_item is None:
            self.current_message_item = UniversalItem(
                item_id=_next_item_id(),
                kind="message",
                role="assistant",
                status="in_progress",
                content=[],
            )
            self.accumulated_text = ""
            events.append(
                UniversalEvent(
                    type="item.started",
                    timestamp=_ts(),
                    session_id=self.session_id,
                    item=self.current_message_item,
                )
            )

        # 累积文本，供 finalize 回填
        self.accumulated_text += block.text
        events.append(
            UniversalEvent(
                type="item.delta",
                timestamp=_ts(),
                session_id=self.session_id,
                item_id=self.current_message_item.item_id,
                delta=ContentDelta(type="text", text=block.text),
            )
        )
        return events

    # ── tool_call -> finalize message + item.started(tool_call) ──

    def _on_tool_call(self, update: ToolCallStart) -> list[UniversalEvent]:
        events: list[UniversalEvent] = []

        # 先 finalize 正在进行的 text message（对标 TS currentMessageItem flush）
        events.extend(self._finalize_current_message())

        item_id = _next_item_id()
        # 稳定工具名：优先 kind，其次 title（agent 子类可 override 提取 _meta）
        name = self._resolve_tool_name(update)
        status = update.status or "pending"
        self.active_tool_calls[update.tool_call_id] = _ActiveToolCall(
            item_id=item_id,
            title=name,
            raw_input=update.raw_input,
            last_status=status,
        )

        tool_item = UniversalItem(
            item_id=item_id,
            kind="tool_call",
            role="assistant",
            status="in_progress",
            content=[
                ContentPart(
                    type="tool_call",
                    call_id=update.tool_call_id,
                    name=name,
                    arguments=_stringify_raw(update.raw_input),
                )
            ],
        )
        events.append(
            UniversalEvent(
                type="item.started",
                timestamp=_ts(),
                session_id=self.session_id,
                item=tool_item,
            )
        )

        # Terminal-on-start：ACP 允许 tool_call 已带 terminal status 且无后续
        # tool_call_update。直接走 update 的 terminal 处理路径，否则 item 会
        # 永远 in_progress（对标 TS terminal-on-start 补偿）。
        if status in ("completed", "failed"):
            events.extend(self._emit_tool_terminal(update, update.tool_call_id))
        return events

    # ── tool_call_update -> 终态时 item.completed(tool_call + tool_result) ──

    def _on_tool_call_update(self, update: ToolCallProgress) -> list[UniversalEvent]:
        tracked = self.active_tool_calls.get(update.tool_call_id)
        # spec: omitted status = unchanged，从跟踪状态继承
        effective_status = (
            update.status or tracked.last_status
            if tracked
            else (update.status or "in_progress")
        )

        if tracked:
            if update.raw_input is not None:
                tracked.raw_input = update.raw_input
            updated_name = self._resolve_tool_name(update)
            if updated_name:
                tracked.title = updated_name
            tracked.last_status = effective_status

        # 非 terminal 直接丢弃（对标 TS break）
        if effective_status not in ("completed", "failed"):
            return []
        # 已 emit 过 terminal，抑制后续重复（spec-violating but defensive）
        if tracked and tracked.terminal_emitted:
            return []
        if tracked:
            tracked.terminal_emitted = True
        return self._emit_tool_terminal(update, update.tool_call_id)

    def _emit_tool_terminal(self, update: Any, tool_call_id: str) -> list[UniversalEvent]:
        """emit tool_call item.completed + tool_result item.completed。

        对标 TS tool_call_update terminal 分支。从 tracked + update 合并出
        最终 name / raw_input / raw_output / status。
        """
        tracked = self.active_tool_calls.pop(tool_call_id, None)
        if tracked is not None:
            item_id = tracked.item_id
            final_name = self._resolve_tool_name(update) or tracked.title
            final_raw_input = _coalesce(getattr(update, "raw_input", None), tracked.raw_input)
        else:
            item_id = _next_item_id()
            final_name = self._resolve_tool_name(update)
            final_raw_input = getattr(update, "raw_input", None)

        status = getattr(update, "status", None) or (
            tracked.last_status if tracked else "completed"
        )
        is_error = status == "failed"

        events: list[UniversalEvent] = []

        # 1) tool_call item.completed
        events.append(
            UniversalEvent(
                type="item.completed",
                timestamp=_ts(),
                session_id=self.session_id,
                item=UniversalItem(
                    item_id=item_id,
                    kind="tool_call",
                    role="assistant",
                    status="completed" if not is_error else "failed",
                    content=[
                        ContentPart(
                            type="tool_call",
                            call_id=tool_call_id,
                            name=final_name,
                            arguments=_stringify_raw(final_raw_input),
                        )
                    ],
                ),
            )
        )

        # 2) tool_result item.completed
        output = self._extract_tool_output(update)
        events.append(
            UniversalEvent(
                type="item.completed",
                timestamp=_ts(),
                session_id=self.session_id,
                item=UniversalItem(
                    item_id=_next_item_id(),
                    kind="tool_result",
                    role="tool",
                    status="failed" if is_error else "completed",
                    content=[
                        ContentPart(
                            type="tool_result",
                            call_id=tool_call_id,
                            output=output,
                            is_error=is_error,
                        )
                    ],
                ),
            )
        )
        return events

    # ── usage_update: 暂存，buildStats 用 ─────────────────

    def _on_usage(self, update: UsageUpdate) -> list[UniversalEvent]:
        self.last_usage_update = update
        return []

    # ── 静默丢弃的 update 类型 ────────────────────────────

    def _on_thought(self, update: AgentThoughtChunk) -> list[UniversalEvent]:
        # 内部推理，不对前端 surface
        return []

    def _on_user_message(self, update: UserMessageChunk) -> list[UniversalEvent]:
        # 回声消息，不 surface（前端已展示用户输入）
        return []

    def _on_plan(self, update: Any) -> list[UniversalEvent]:
        # planning 步骤暂无 UI 映射
        return []

    def _on_meta_update(self, update: Any) -> list[UniversalEvent]:
        # available_commands / current_mode / session_info / config_option
        # 仅影响 UI chrome 不产生 item 事件
        return []

    # ── agent 子类 override 钩子 ──────────────────────────

    def _resolve_tool_name(self, update: Any) -> str:
        """提取稳定的工具名。

        默认：优先 ACP ``kind``（execute/read/edit/...），其次 ``title``。
        agent 子类可 override 从 ``_meta`` 提取私有工具名（如 goose 的
        ``_meta.goose.toolCall.toolName``）。
        """
        kind = getattr(update, "kind", None)
        if kind:
            return str(kind)
        title = getattr(update, "title", None)
        return title or ""

    def _extract_tool_output(self, update: Any) -> str:
        """从 tool_call_update 提取 output 文本。

        默认：优先 raw_output，其次 content blocks 文本。agent 子类可
        override 处理图片生成等特殊输出（如 codex 的 raw_output.result
        是 MB 级 base64，需替换为摘要）。
        """
        output = _stringify_raw(getattr(update, "raw_output", None))
        if output:
            return output
        content = getattr(update, "content", None) or []
        parts: list[str] = []
        for block in content:
            text = _stringify_content_block(block)
            if text:
                parts.append(text)
        return "\n".join(filter(None, parts))

    # ── turn 结束统计 ─────────────────────────────────────

    def build_stats(self, result: PromptResult | PromptResponse | None = None) -> TurnStats | None:
        """从 last_usage_update + result.usage 构建 TurnStats。

        两个来源：
          - PromptResponse.usage: per-turn input/output/cache tokens
          - UsageUpdate event: cumulative context size + cost

        对标 TS buildStats。codex CLI 不填 PromptResponse.usage，故
        input/output 可能是 0；cost 从 usage_update.cost 取。
        """
        update = self.last_usage_update
        usage = getattr(result, "usage", None) if result is not None else None
        if update is None and usage is None:
            return None

        duration_ms = self.turn_start_ms - _ts() if self.turn_start_ms else 0
        # 取绝对值避免顺序导致的负数
        duration_ms = abs(duration_ms)
        self.turn_start_ms = 0

        cost_amount = 0.0
        if update is not None and update.cost is not None:
            cost_amount = update.cost.amount

        return TurnStats(
            cost_usd=cost_amount,
            duration_ms=duration_ms,
            input_tokens=getattr(usage, "input_tokens", 0) or 0,
            output_tokens=getattr(usage, "output_tokens", 0) or 0,
            cache_read_tokens=getattr(usage, "cached_read_tokens", 0) or 0,
            cache_creation_tokens=getattr(usage, "cached_write_tokens", 0) or 0,
            num_turns=1,
            context_tokens=update.used if update is not None else 0,
            context_window=update.size if update is not None else 0,
        )

    # ── finalize: turn 结束时 flush 未完成的 message ─────

    def finalize(self) -> list[UniversalEvent]:
        """turn 结束时调用，flush 未完成的 current_message_item。

        对标 TS finalize()。active_tool_calls 中残留的非 terminal 调用
        不在此处理（ACP spec 保证 tool_call_update 会带 terminal status）。
        """
        return self._finalize_current_message()

    def _finalize_current_message(self) -> list[UniversalEvent]:
        if self.current_message_item is None:
            return []
        self.current_message_item.status = "completed"
        if self.accumulated_text:
            self.current_message_item.content = [
                ContentPart(type="text", text=self.accumulated_text)
            ]
        event = UniversalEvent(
            type="item.completed",
            timestamp=_ts(),
            session_id=self.session_id,
            item=self.current_message_item,
        )
        self.current_message_item = None
        self.accumulated_text = ""
        return [event]


# ── 模块级辅助 ─────────────────────────────────────────────


def _stringify_raw(raw: Any) -> str:
    """序列化 raw_input / raw_output 为展示字符串。对标 TS stringifyRaw。"""
    if raw is None:
        return ""
    if isinstance(raw, str):
        return raw
    try:
        return json.dumps(raw, ensure_ascii=False, default=str)
    except (TypeError, ValueError):
        return str(raw)


def _stringify_content_block(block: Any) -> str:
    """从 ToolCallUpdate.content 的一个 block 提取文本。

    对标 TS tool_call_update content blocks 提取：content.text / diff /
    terminal 标识。未知 block 返回空串而非报错，避免阻塞 SSE 流。
    """
    block_type = getattr(block, "type", None) or (
        block.get("type") if isinstance(block, dict) else None
    )
    if block_type == "content":
        inner = getattr(block, "content", None) or (
            block.get("content") if isinstance(block, dict) else None
        )
        inner_type = getattr(inner, "type", None) or (
            inner.get("type") if isinstance(inner, dict) else None
        )
        if inner_type == "text":
            return (
                getattr(inner, "text", None)
                or (inner.get("text") if isinstance(inner, dict) else "")
                or ""
            )
        return _stringify_raw(inner)
    if block_type == "diff":
        path = (
            getattr(block, "path", None)
            or (block.get("path") if isinstance(block, dict) else "")
            or ""
        )
        old_text = (
            getattr(block, "old_text", None)
            or (block.get("oldText") if isinstance(block, dict) else "")
            or ""
        )
        new_text = (
            getattr(block, "new_text", None)
            or (block.get("newText") if isinstance(block, dict) else "")
            or ""
        )
        if old_text:
            return f"--- {path}\n+++ {path}\n{old_text}\n->\n{new_text}"
        return f"+++ {path}\n{new_text}"
    if block_type == "terminal":
        terminal_id = (
            getattr(block, "terminal_id", None)
            or (block.get("terminalId") if isinstance(block, dict) else "")
            or ""
        )
        return f"[terminal: {terminal_id}]"
    return ""


def _coalesce(*values: Any) -> Any:
    """返回第一个非 None 的值。"""
    for value in values:
        if value is not None:
            return value
    return None
