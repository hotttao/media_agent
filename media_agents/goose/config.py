"""Goose AgentHooks 实现。

对标 agents/goose/src/server.ts 的 trackExtNotification + recordUsage：
- trackExtNotification 拦截 _goose/unstable/session/update 累计 token
- recordUsage 在 turn 完成后写 ~/.acp-usage/<sessionId>.jsonl，因为 goose SQLite
  不能被 agent-usage sweeper 解析
"""

from __future__ import annotations

import json
import logging
import os
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

import httpx

logger = logging.getLogger(__name__)

HOME = Path(os.environ.get("HOME", "/root"))
WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", "/workspace"))


def _num(v: Any) -> int:
    return int(v) if isinstance(v, (int, float)) and v == v else 0


class GooseHooks:
    """AgentHooks Protocol 的 goose 实现。"""

    def __init__(
        self,
        *,
        cp_url: str | None,
        workspace_id: str | None,
        goose_path_root: str | None = None,
    ) -> None:
        self._cp_url = cp_url
        self._workspace_id = workspace_id
        self._goose_path_root = goose_path_root
        # 平台 session id -> 累计 token（input/output）
        # 注：bridge 把 ext notification 的 native id 替换成 platform id 后回调
        self._accumulated: dict[str, tuple[int, int]] = {}

    # ── trackExtNotification：goose 特有，bridge 调用 ─────────────

    def track_ext_notification(
        self, platform_session_id: str, method: str, params: dict[str, Any]
    ) -> None:
        if method != "_goose/unstable/session/update":
            return
        update = params.get("update") or {}
        if update.get("sessionUpdate") != "usage_update":
            return
        inp = _num(update.get("accumulatedInputTokens"))
        out = _num(update.get("accumulatedOutputTokens"))
        if inp == 0 and out == 0:
            return
        self._accumulated[platform_session_id] = (inp, out)

    # ── AgentHooks 协议实现 ────────────────────────────────────────

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        if not self._cp_url or not self._workspace_id:
            return []
        headers = {
            "X-Workspace-ID": self._workspace_id,
            "X-Agent-ID": self._workspace_id,
        }
        if session_token:
            headers["X-Session-Token"] = session_token
        return [
            {
                "type": "http",
                "name": "tos-platform",
                "url": f"{self._cp_url}/mcp",
                "headers": headers,
            }
        ]

    def has_mcp_servers(self) -> bool:
        # 简化版：goose 总是挂 platform MCP
        return bool(self._cp_url and self._workspace_id)

    async def load_config(self) -> bool:
        if not self._cp_url or not self._workspace_id:
            logger.info("Config skipped: CP_URL/WORKSPACE_ID unset")
            return False
        url = f"{self._cp_url}/_cp/workspaces/{self._workspace_id}/config"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url)
            if not resp.is_success:
                logger.error("Config fetch failed %s %s", resp.status_code, resp.reason_phrase)
                return False
            config = resp.json()
        (WORKSPACE_DIR / "runtime.json").write_text(
            json.dumps(
                {
                    "model": config.get("model", ""),
                    "provider_type": config.get("provider_type", ""),
                    "base_url": config.get("base_url", ""),
                    "small_model": config.get("small_model", ""),
                }
            ),
            encoding="utf-8",
        )
        # 写 AGENTS.md 系统提示（对标 config.prompt_content）
        prompt = config.get("prompt_content") or config.get("system_prompt") or ""
        (WORKSPACE_DIR / "AGENTS.md").write_text(prompt, encoding="utf-8")
        logger.info("Config written: model=%s", config.get("model"))
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        logger.info("Skills load skipped (not implemented)")
        return True, []

    async def load_credentials(self) -> bool:
        logger.info("Credentials load skipped (not implemented)")
        return False

    async def restart_bridge(self) -> None:
        # goose 没有像 codex 那样要刷 OPENAI_API_KEY env（provider config 写在 session config）
        return

    def record_usage(self, session_id: str, usage: Any) -> None:
        """对标 agents/goose/src/server.ts recordUsage：写 JSONL 到 ~/.acp-usage/。"""
        acc = self._accumulated.get(session_id)
        model = _load_runtime_model()
        payload: dict[str, Any] | None = None
        if acc:
            payload = {
                "ts": datetime.now(UTC).isoformat(),
                "model": model,
                "accumulated_input_tokens": acc[0],
                "accumulated_output_tokens": acc[1],
            }
        elif isinstance(usage, dict):
            inp = _num(usage.get("inputTokens"))
            out = _num(usage.get("outputTokens"))
            if inp == 0 and out == 0:
                return
            payload = {
                "ts": datetime.now(UTC).isoformat(),
                "model": model,
                "input_tokens": inp,
                "output_tokens": out,
                "total_tokens": _num(usage.get("totalTokens")) or inp + out,
            }
        if not payload:
            return
        try:
            acp_dir = HOME / ".acp-usage"
            acp_dir.mkdir(parents=True, exist_ok=True)
            with (acp_dir / f"{session_id}.jsonl").open("a", encoding="utf-8") as f:
                f.write(json.dumps(payload) + "\n")
        except OSError:
            logger.exception("record_usage write failed session=%s", session_id)


def _load_runtime_model() -> str | None:
    try:
        text = (WORKSPACE_DIR / "runtime.json").read_text(encoding="utf-8")
        return cast(str | None, json.loads(text).get("model"))
    except (FileNotFoundError, json.JSONDecodeError):
        return None
