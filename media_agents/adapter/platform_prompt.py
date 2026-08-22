"""Platform-managed prompt rendering shared by agent packages."""

from __future__ import annotations

import re
from collections.abc import Iterable, Mapping
from importlib import resources
from pathlib import Path
from typing import Literal, NotRequired, TypedDict

AgentKind = Literal["claude-code", "codex", "goose"]
MemoryAccess = Literal["read_only", "read_write"]
TemplateContext = dict[str, object]

_SECTION_RE = re.compile(r"{{#([A-Za-z0-9_]+)}}(.*?){{/\1}}", re.DOTALL)
_TRIPLE_VAR_RE = re.compile(r"{{{\s*([A-Za-z0-9_]+)\s*}}}")
_VAR_RE = re.compile(r"{{\s*([A-Za-z0-9_]+)\s*}}")


class MemoryAttachmentView(TypedDict):
    store_id: str
    store_name: str
    store_description: str
    access: MemoryAccess
    instructions: str
    index_content: NotRequired[str | None]


def normalize_memory_attachments(raw: object) -> list[MemoryAttachmentView]:
    """Normalize CP memory_attachments into prompt-renderable views."""
    if not isinstance(raw, list):
        return []
    attachments: list[MemoryAttachmentView] = []
    for item in raw:
        if not isinstance(item, Mapping):
            continue
        store_id = _str(item.get("store_id") or item.get("storeId"))
        store_name = _str(item.get("store_name") or item.get("storeName"))
        if not store_id or not store_name:
            continue
        access = item.get("access")
        attachments.append(
            {
                "store_id": store_id,
                "store_name": store_name,
                "store_description": _str(
                    item.get("store_description") or item.get("storeDescription")
                ),
                "access": access if access in {"read_only", "read_write"} else "read_only",
                "instructions": _str(item.get("instructions")),
                "index_content": _optional_str(
                    item.get("index_content") or item.get("indexContent")
                ),
            }
        )
    return attachments


def render_platform_prompt(
    *,
    agent_kind: AgentKind,
    workspace_id: str | None,
    user_name: str | None = None,
    memory_attachments: Iterable[MemoryAttachmentView] | None = None,
    template: str | None = None,
) -> str:
    """Render the platform-managed global prompt from ``platform_prompt.md``."""
    attachments = [_memory_attachment_context(a) for a in memory_attachments or []]
    context: TemplateContext = {
        "workspaceId": workspace_id or "unknown",
        "userName": user_name or "",
        "claudeCode": agent_kind == "claude-code",
        "codex": agent_kind == "codex",
        "goose": agent_kind == "goose",
        "hasMemoryAttachments": bool(attachments),
        "memoryAttachments": attachments,
    }
    source = template if template is not None else load_platform_prompt_template()
    return render_template(source, context)


def write_platform_prompt(
    *,
    agent_kind: AgentKind,
    target_dir: str | Path,
    filename: str,
    workspace_id: str | None,
    user_name: str | None = None,
    memory_attachments: Iterable[MemoryAttachmentView] | None = None,
) -> str:
    prompt = render_platform_prompt(
        agent_kind=agent_kind,
        workspace_id=workspace_id,
        user_name=user_name,
        memory_attachments=memory_attachments,
    )
    target = Path(target_dir).expanduser()
    target.mkdir(parents=True, exist_ok=True)
    (target / filename).write_text(prompt, encoding="utf-8")
    return prompt


def load_platform_prompt_template() -> str:
    return resources.files("adapter").joinpath("platform_prompt.md").read_text(encoding="utf-8")


def render_template(template: str, context: Mapping[str, object]) -> str:
    """Render the Mustache subset used by ``platform_prompt.md``.

    Supported syntax matches the platform prompt template: variables, triple-stache
    variables, truthy sections, and list sections.
    """

    def replace_section(match: re.Match[str]) -> str:
        name = match.group(1)
        body = match.group(2)
        value = context.get(name)
        if isinstance(value, list):
            return "".join(
                render_template(body, {**context, **item})
                for item in value
                if isinstance(item, Mapping)
            )
        if value:
            return render_template(body, context)
        return ""

    previous = None
    rendered = template
    while previous != rendered:
        previous = rendered
        rendered = _SECTION_RE.sub(replace_section, rendered)
    rendered = _TRIPLE_VAR_RE.sub(lambda m: _render_value(context.get(m.group(1))), rendered)
    return _VAR_RE.sub(lambda m: _render_value(context.get(m.group(1))), rendered)


def _memory_attachment_context(attachment: MemoryAttachmentView) -> TemplateContext:
    index = (attachment.get("index_content") or "").strip()
    instructions = attachment["instructions"]
    return {
        "storeId": attachment["store_id"],
        "storeName": attachment["store_name"],
        "storeDescription": attachment["store_description"],
        "access": attachment["access"],
        "instructions": instructions,
        "hasInstructions": bool(instructions.strip()),
        "hasIndex": bool(index),
        "indexContent": index,
    }


def _render_value(value: object) -> str:
    return value if isinstance(value, str) else ""


def _str(value: object) -> str:
    return value if isinstance(value, str) else ""


def _optional_str(value: object) -> str | None:
    return value if isinstance(value, str) else None
