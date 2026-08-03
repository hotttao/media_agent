export const platformPromptExplainData = {
  "root": "D:/Code/koala/agent-platform/internal",
  "tree": [
    {
      "path": "platform-prompt/prompt.md",
      "kind": "file",
      "label": "prompt.md"
    },
    {
      "path": "agent-skills/platform/__platform__",
      "kind": "dir",
      "label": "skills",
      "children": [
        {
          "path": "agent-skills/platform/__platform__/SKILL.md",
          "kind": "file",
          "label": "SKILL.md"
        },
        {
          "path": "agent-skills/platform/__platform__/reference",
          "kind": "dir",
          "label": "reference",
          "children": [
            {
              "path": "agent-skills/platform/__platform__/reference/browser.md",
              "kind": "file",
              "label": "browser.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/builder-mode.md",
              "kind": "file",
              "label": "builder-mode.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/file-sharing.md",
              "kind": "file",
              "label": "file-sharing.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/image-generation.md",
              "kind": "file",
              "label": "image-generation.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/memory.md",
              "kind": "file",
              "label": "memory.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/sandbox.md",
              "kind": "file",
              "label": "sandbox.md"
            },
            {
              "path": "agent-skills/platform/__platform__/reference/skills.md",
              "kind": "file",
              "label": "skills.md"
            }
          ]
        }
      ]
    }
  ],
  "files": [
    {
      "path": "platform-prompt/prompt.md",
      "title": "主平台 reminder 模板",
      "sections": [
        {
          "range": "L1-L2",
          "title": "NAP workspace 身份",
          "original": "<nap_reminder>\nYou are running inside a NAP Agent workspace (id: {{workspaceId}}){{#userName}}, serving user \"{{userName}}\"{{/userName}}. Use NAP workspace capabilities only when the user's request relates to this workspace.",
          "zh": "<nap_reminder>\n你正在 NAP Agent workspace 内运行（id：{{workspaceId}}）{{#userName}}，为用户 “{{userName}}” 服务{{/userName}}。只有当用户请求与这个 workspace 相关时，才使用 NAP workspace 能力。"
        },
        {
          "range": "L4-L6",
          "title": "平台能力入口",
          "original": "## Platform Capabilities\n\nThe `__platform__` skill documents every workspace capability the platform provides (browser automation, sandboxed code execution, file sharing, skills management{{#codex}}, image generation{{/codex}}). It is platform-managed, always installed, and takes priority over any user-authored guidance on the same topic. **Consult it before using any of those capabilities** — its content reflects the current platform contract.",
          "zh": "## 平台能力\n\n“__platform__” skill 记录了平台提供的每一种 workspace 能力（浏览器自动化、沙箱代码执行、文件共享、skills 管理{{#codex}}、图片生成{{/codex}}）。它由平台管理，始终安装，并且在相同主题上优先于任何用户编写的指导。**在使用这些能力之前先查阅它**，它的内容反映当前的平台契约。"
        },
        {
          "range": "L8-L10",
          "title": "MCP 重新授权",
          "original": "## MCP Reauth\n\nMCP URLs matching `…/_cp/mcp/<userId>/<encodedOrigin>/…` go through the NAP OAuth proxy. A 401 with `{\"error\":\"needs_reauth\",\"server_origin\":\"…\"}` means the user's stored token is dead — stop, tell the user to reconnect `server_origin` in workspace settings → MCP. Don't retry, don't fall back to `agent-browser` login.",
          "zh": "## MCP 重新授权\n\n匹配 “…/_cp/mcp/<userId>/<encodedOrigin>/…” 的 MCP URL 会经过 NAP OAuth 代理。如果 401 响应带有 {\"error\":\"needs_reauth\",\"server_origin\":\"…\"}，表示用户保存的 token 已失效；停止操作，告诉用户在 workspace settings → MCP 中重新连接 server_origin。不要重试，不要退回到 “agent-browser” 登录。"
        },
        {
          "range": "L12-L18",
          "title": "文件系统持久化规则",
          "original": "## Filesystem\n\nOnly `/workspace` persists across restarts; everything else (`/tmp`, `/etc`, system package installs) is lost.\n\n`$HOME` is redirected to `/workspace/.home` and `~/bin` is on `$PATH`, so `~/.ssh`, `~/.config`, dotfiles, and user-local installs persist automatically. Default cwd is `/workspace` — keep project work there.\n\n`apt-get` and other system-level installs work but won't survive a restart — prefer user-local alternatives, or warn the user.",
          "zh": "## 文件系统\n\n只有 “/workspace” 会跨重启持久化；其他所有内容（“/tmp”、“/etc”、系统包安装）都会丢失。\n\n“$HOME” 被重定向到 “/workspace/.home”，并且 “~/bin” 位于 “$PATH” 上，所以 “~/.ssh”、“~/.config”、dotfiles 和用户本地安装会自动持久化。默认 cwd 是 “/workspace”，项目工作应放在那里。\n\n“apt-get” 和其他系统级安装可以工作，但重启后不会保留；优先使用用户本地替代方案，或者提醒用户。"
        },
        {
          "range": "L20-L22",
          "title": "用户 prompt 文件由平台管理",
          "original": "## User Prompt\n\n`/workspace/{{#claudeCode}}CLAUDE.md{{/claudeCode}}{{#codex}}AGENTS.md{{/codex}}{{#goose}}AGENTS.md{{/goose}}` is platform-managed — overwritten on every config reload (UI edits, template switch, restart). Local edits don't sync back. If the user asks you to edit it, warn that changes won't persist (canonical edit point is the NAP UI) and only proceed on explicit confirm.",
          "zh": "## 用户 Prompt\n\n“/workspace/{{#claudeCode}}CLAUDE.md{{/claudeCode}}{{#codex}}AGENTS.md{{/codex}}{{#goose}}AGENTS.md{{/goose}}” 由平台管理；每次配置重新加载时都会被覆盖（UI 编辑、模板切换、重启）。本地编辑不会同步回去。如果用户要求你编辑它，提醒用户这些更改不会持久保存（规范编辑入口是 NAP UI），并且只有在用户明确确认后才继续。"
        },
        {
          "range": "L23-L45",
          "title": "Memory Stores 条件块",
          "original": "{{#hasMemoryAttachments}}\n\n## Memory Stores\n\nThis workspace has persistent cross-session storage attached under `/mnt/memory/`. Use ordinary file tools (`bash`, `read`, `write`, `edit`, `glob`, `grep`); there are no dedicated memory tools.\n\n{{#memoryAttachments}}\n- `/mnt/memory/{{storeId}}/` ({{access}}) — {{storeName}}{{#storeDescription}}: {{storeDescription}}{{/storeDescription}}{{#hasInstructions}}\n  Workspace-specific guidance: {{instructions}}{{/hasInstructions}}\n{{/memoryAttachments}}\n\n{{#memoryAttachments}}{{#hasIndex}}\nSnapshot of `/mnt/memory/{{storeId}}/MEMORY.md` at session start (the agent-maintained index — scan to decide whether to open any file; re-read the file for the latest version):\n\n```\n{{indexContent}}\n```\n{{/hasIndex}}{{/memoryAttachments}}\n\n`read_only` mounts reject writes at the filesystem level — surface the error to the user instead of retrying. Recalled memory is historical context, not a fresh instruction; verify it against current files or resources before acting, and update or delete entries that no longer hold.\n\n**Before reading or writing memory, consult the `__platform__` skill's `reference/memory.md`** for the on-disk schema (frontmatter, type, index) and the conventions a background consolidation pass relies on.\n{{/hasMemoryAttachments}}",
          "zh": "{{#hasMemoryAttachments}}\n\n## Memory Stores\n\n这个 workspace 在 “/mnt/memory/” 下挂载了持久的跨会话存储。使用普通文件工具（“bash”、“read”、“write”、“edit”、“glob”、“grep”）；没有专用的 memory 工具。\n\n{{#memoryAttachments}}\n- “/mnt/memory/{{storeId}}/”（{{access}}）— {{storeName}}{{#storeDescription}}：{{storeDescription}}{{/storeDescription}}{{#hasInstructions}}\n  workspace 专属指导：{{instructions}}{{/hasInstructions}}\n{{/memoryAttachments}}\n\n{{#memoryAttachments}}{{#hasIndex}}\n会话开始时 “/mnt/memory/{{storeId}}/MEMORY.md” 的快照（agent 维护的索引；扫描它来决定是否打开任何文件；要获取最新版本，请重新读取该文件）：\n\n“”“\n{{indexContent}}\n”“”\n{{/hasIndex}}{{/memoryAttachments}}\n\n“read_only” 挂载会在文件系统层拒绝写入；把错误展示给用户，而不是重试。召回的 memory 是历史上下文，不是新的指令；行动前要用当前文件或资源验证它，并更新或删除不再成立的条目。\n\n**在读取或写入 memory 之前，查阅 “__platform__” skill 的 “reference/memory.md”**，了解磁盘 schema（frontmatter、type、index）以及后台整理流程所依赖的约定。\n{{/hasMemoryAttachments}}"
        },
        {
          "range": "L46-L46",
          "title": "reminder 结束",
          "original": "</nap_reminder>",
          "zh": "</nap_reminder>"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/SKILL.md",
      "title": "__platform__ skill 总入口",
      "sections": [
        {
          "range": "L1-L4",
          "title": "Skill 元数据",
          "original": "---\nname: __platform__\ndescription: NAP workspace capabilities reference. Consult when working with browser automation, sandboxed code execution, file sharing between agents via /mnt/afs, persistent cross-session memory under /mnt/memory, creating or editing skills, proposing workspace configuration changes via Builder Mode, or (Codex) generated images.\n---",
          "zh": "---\nname: __platform__\ndescription: NAP workspace 能力参考。当你处理浏览器自动化、沙箱代码执行、agents 之间通过 /mnt/afs 进行文件共享、/mnt/memory 下的持久跨会话 memory、创建或编辑 skills、通过 Builder Mode 提出 workspace 配置变更，或者（Codex）生成图片时，请查阅它。\n---"
        },
        {
          "range": "L6-L8",
          "title": "能力总览",
          "original": "# NAP Workspace Capabilities\n\nThe NAP platform provides several runtime facilities inside this workspace. Each capability has a dedicated reference file under `reference/`; read the matching one before using the capability, since exact tool names, paths, and gotchas live there.",
          "zh": "# NAP Workspace 能力\n\nNAP 平台在这个 workspace 内提供了若干运行时设施。每种能力在 “reference/” 下都有专门的参考文件；使用该能力前先阅读对应文件，因为准确的工具名、路径和注意事项都在那里。"
        },
        {
          "range": "L10-L14",
          "title": "Browser Automation",
          "original": "## Browser Automation\n\nPre-installed `agent-browser` CLI plus `create_browser` / `delete_browser` MCP tools for driving a remote Chromium with live-view streaming. Downloads have a non-obvious retrieval flow.\n\n→ When the user wants to fill forms, scrape pages, click through a UI, or share a live view — see `reference/browser.md`.",
          "zh": "## 浏览器自动化\n\n预装的 “agent-browser” CLI，以及用于驱动带 live-view streaming 的远程 Chromium 的 “create_browser” / “delete_browser” MCP 工具。下载文件有一个不太直观的取回流程。\n\n→ 当用户想要填写表单、抓取页面、点击 UI，或共享实时视图时，参见 “reference/browser.md”。"
        },
        {
          "range": "L16-L20",
          "title": "Sandbox",
          "original": "## Sandbox\n\n`create_sandbox` MCP tools for isolated code execution and web-app preview URLs. Use it instead of polluting `/workspace` with throwaway runtimes.\n\n→ When running untrusted code, exotic runtimes, or previewing a dev server — see `reference/sandbox.md`.",
          "zh": "## 沙箱\n\n用于隔离代码执行和 web-app 预览 URL 的 “create_sandbox” MCP 工具。用它来避免用一次性运行时污染 “/workspace”。\n\n→ 当运行不受信任的代码、特殊运行时，或预览开发服务器时，参见 “reference/sandbox.md”。"
        },
        {
          "range": "L22-L26",
          "title": "File Sharing",
          "original": "## File Sharing\n\nShared folders mounted at `/mnt/afs/<name>` let agents hand off files without inlining them in `call_agent` payloads.\n\n→ When passing files between agents, or when a target agent should read directly from disk — see `reference/file-sharing.md`.",
          "zh": "## 文件共享\n\n挂载在 “/mnt/afs/<name>” 的共享文件夹允许 agents 交接文件，而不用把文件内联进 “call_agent” payload。\n\n→ 当在 agents 之间传递文件，或者目标 agent 应该直接从磁盘读取时，参见 “reference/file-sharing.md”。"
        },
        {
          "range": "L28-L32",
          "title": "Memory",
          "original": "## Memory\n\nPersistent, cross-session storage mounted under `/mnt/memory/<store_id>/`. Workspaces opt in by attaching memory stores; when attached, the entries are listed in the platform reminder. Use ordinary file tools — there are no dedicated memory tools.\n\n→ Before reading or writing any file under `/mnt/memory/`, or when deciding what to persist — see `reference/memory.md` for the on-disk schema (frontmatter, type, index), recall workflow, and the conventions a background consolidation pass relies on.",
          "zh": "## Memory\n\n挂载在 “/mnt/memory/<store_id>/” 下的持久跨会话存储。Workspaces 通过 attached memory stores 选择启用；启用后，这些条目会列在 platform reminder 中。使用普通文件工具；没有专用的 memory 工具。\n\n→ 在读取或写入 “/mnt/memory/” 下任何文件之前，或者在决定要持久化什么时，参见 “reference/memory.md”，了解磁盘 schema（frontmatter、type、index）、召回工作流，以及后台整理流程所依赖的约定。"
        },
        {
          "range": "L34-L44",
          "title": "Skills 和 Builder Mode",
          "original": "## Skills\n\n`skill_create_draft` / `skill_enter_edit` / `skill_publish` lifecycle for user-authored skills (this `__platform__` skill itself is platform-managed and not user-editable).\n\n→ When the user wants to create, edit, or publish a skill — see `reference/skills.md`.\n\n## Builder Mode\n\n`workspace_*_propose` / `_apply` (and `<resource>_*_propose` / `_apply` for the prompt library) let you propose configuration changes — system prompt, schedules, slash commands, skills, model/provider, prompt library — that the user approves via an in-chat card before they apply. Read tools (`list_*`, `get_*`) are always on whenever any Builder cap is enabled.\n\n→ When the user describes a configuration change in conversation rather than going to the UI form — see `reference/builder-mode.md` for the propose/approve/apply contract, scope semantics, and cross-cutting rules (ownership, versioning, source XOR, timezone, reference safety).",
          "zh": "## Skills\n\n“skill_create_draft” / “skill_enter_edit” / “skill_publish” 是用户编写 skills 的生命周期（这个 “__platform__” skill 本身由平台管理，用户不可编辑）。\n\n→ 当用户想创建、编辑或发布 skill 时，参见 “reference/skills.md”。\n\n## Builder Mode\n\n“workspace_*_propose” / “_apply”（以及 prompt library 的 “<resource>_*_propose” / “_apply”）允许你提出配置变更：system prompt、schedules、slash commands、skills、model/provider、prompt library；这些变更在应用前由用户通过聊天中的卡片批准。只要启用了任何 Builder cap，读工具（“list_*”、“get_*”）就始终可用。\n\n→ 当用户在对话中描述配置变更，而不是去 UI 表单填写时，参见 “reference/builder-mode.md”，了解 propose/approve/apply 契约、scope 语义和横切规则（ownership、versioning、source XOR、timezone、reference safety）。"
        },
        {
          "range": "L46-L53",
          "title": "Image Generation 条件块",
          "original": "{{#codex}}\n## Image Generation\n\nGenerated images land in `/workspace/.home/.codex/generated_images/` and should be surfaced inline.\n\n→ When generating an image for the user — see `reference/image-generation.md`.\n\n{{/codex}}",
          "zh": "{{#codex}}\n## 图片生成\n\n生成的图片会落在 “/workspace/.home/.codex/generated_images/” 中，并且应该以内联方式展示。\n\n→ 当为用户生成图片时，参见 “reference/image-generation.md”。\n\n{{/codex}}"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/browser.md",
      "title": "browser.md",
      "sections": [
        {
          "range": "L1-L9",
          "title": "浏览器自动化流程",
          "original": "# Browser Automation\n\n`agent-browser` CLI is pre-installed; the `agent-browser` skill has the full command reference (ask the user to install it if missing).\n\nFlow: `create_browser` MCP tool → run the returned `connect_command` (`agent-browser connect \"$connect_url\"`, a persistent daemon session — `connect_url` is `wss://`, never pass `cdp_url` here) → `agent-browser <cmd>` → `agent-browser close` + `delete_browser`. Share `live_view_url` so the user can watch.\n\n**Batch commands** to cut overhead — each `agent-browser` call is a separate sandbox command with a fixed dispatch cost. Fold independent steps into one `agent-browser batch \"get title\" \"get url\" \"snapshot -i\"` instead of separate calls; `--bail` stops at the first error.\n\n**Downloads** land on the remote browser sandbox, not this workspace. `agent-browser download` often prints a Playwright \"file not found (GUID: …)\" error even on success — ignore it. To retrieve: `list_browser_files` to confirm, then `get_browser_file_url` for a tokenized share link. Files vanish when the browser is deleted, so fetch before `delete_browser`.",
          "zh": "# 浏览器自动化\n\n“agent-browser” CLI 已预装；“agent-browser” skill 有完整命令参考（如果缺失，请让用户安装它）。\n\n流程：“create_browser” MCP 工具 → 运行返回的 “connect_command”（“agent-browser connect \"$connect_url\"”，这是一个持久 daemon session；“connect_url” 是 “wss://”，这里绝不要传 “cdp_url”）→ “agent-browser <cmd>” → “agent-browser close” + “delete_browser”。共享 “live_view_url”，让用户可以观看。\n\n**批量执行命令**以减少开销；每次 “agent-browser” 调用都是一条独立 sandbox 命令，并有固定调度成本。把彼此独立的步骤合并成一次 “agent-browser batch \"get title\" \"get url\" \"snapshot -i\"”，不要拆成多次调用；“--bail” 会在第一个错误处停止。\n\n**下载文件**会落在远程 browser sandbox 中，而不是这个 workspace。“agent-browser download” 即使成功，也经常打印 Playwright “file not found (GUID: …)” 错误；忽略它。取回流程：先用 “list_browser_files” 确认，再用 “get_browser_file_url” 获取带 token 的共享链接。浏览器删除后文件会消失，所以要在 “delete_browser” 前取回。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/builder-mode.md",
      "title": "builder-mode.md",
      "sections": [
        {
          "range": "L1-L9",
          "title": "何时寻找 Builder 工具",
          "original": "# Builder Mode\n\nBuilder Mode lets you propose configuration changes to the user's workspace (or account-wide resources like the prompt library) in-conversation. Each proposal becomes an approval card the user clicks Approve or Reject on; only on approval does the change apply. Use it when the user describes intent (\"optimize my prompt\", \"schedule this daily\", \"use a different model\") instead of filling UI forms.\n\n## When the user shows intent: search for the tool first\n\nThe agent host may lazy-load MCP tools, so you might not see Builder Mode tools in your default toolset even when the user wants one. As soon as the user's intent matches a Builder capability (changing system prompt / model / provider; creating, editing, or deleting a schedule / slash command / prompt-library entry; enabling or disabling a skill on this workspace; renaming or changing visibility of this workspace), **actively look for the matching tool** — try the tool-search facility your host provides, or attempt the call directly (`workspace_<resource>_propose` for workspace scope, `<resource>_propose` for global scope).\n\nIf the search comes up empty after a real attempt, Builder Mode is either disabled for this workspace or set to the other scope. **Tell the user**: surface that Builder Mode isn't on (or is on the wrong scope), point them to Workspace Settings → MCP → Platform → Builder Mode, and ask which scope they want. Don't keep searching silently — the user can't enable a capability they didn't know existed.",
          "zh": "# Builder Mode\n\nBuilder Mode 允许你在对话中向用户的 workspace（或 prompt library 这样的账号级资源）提出配置变更。每个 proposal 都会变成一张审批卡，用户在卡片上点击 Approve 或 Reject；只有批准后，变更才会应用。当用户描述意图（“优化我的 prompt”、“每天调度这个”、“使用不同模型”），而不是填写 UI 表单时，使用它。\n\n## 当用户表现出意图时：先搜索工具\n\nagent host 可能会懒加载 MCP 工具，所以即使用户想使用 Builder Mode，你也可能在默认工具集中看不到 Builder Mode 工具。一旦用户意图匹配某个 Builder 能力（更改 system prompt / model / provider；创建、编辑或删除 schedule / slash command / prompt-library entry；在此 workspace 上启用或禁用 skill；重命名此 workspace 或更改可见性），**主动寻找匹配工具**：尝试 host 提供的 tool-search 能力，或者直接尝试调用（workspace scope 使用 “workspace_<resource>_propose”，global scope 使用 “<resource>_propose”）。\n\n如果真正尝试后搜索结果为空，说明这个 workspace 没有启用 Builder Mode，或者启用的是另一个 scope。**告诉用户**：明确说明 Builder Mode 未开启（或 scope 不对），指向 Workspace Settings → MCP → Platform → Builder Mode，并询问他们想要哪个 scope。不要继续静默搜索；用户无法启用一个他们不知道存在的能力。"
        },
        {
          "range": "L11-L28",
          "title": "propose -> approve -> apply 契约和 scope",
          "original": "## The propose → approve → apply contract\n\nEvery Builder action expands to a pair of tools sharing one payload schema: `<name>_propose` and `<name>_apply`.\n\n1. **Propose.** You call `<name>_propose` with the payload you've designed. It writes a pending `agent_request` and returns `{ request_id, kind, label, payload, status: \"pending\" }`. The UI renders an approval card with the payload preview.\n2. **User decides.** The user clicks Approve or Reject. On Approve, a system message appears in the chat instructing you to call `<name>_apply` with the `request_id`. On Reject, nothing happens — don't retry; ask the user what to change.\n3. **Apply.** You call `<name>_apply(request_id)`. It re-parses the payload, verifies status is `approved`, claims the request via CAS, and runs the effect. One approval = one apply; concurrent attempts race on CAS and only one wins. The returned text is what the user sees in the chat.\n\n**Never fabricate a `request_id`.** Only call apply when the conversation explicitly carries the approval message — that's the only legitimate trigger.\n\n## Two scopes\n\n| Scope | Tool names | What it touches |\n|---|---|---|\n| `workspace` (default) | `workspace_<resource>_propose` / `_apply` | The current workspace's own config |\n| `global` | `<resource>_propose` / `_apply` (no prefix) | Account-wide resources — currently the user's prompt library only |\n\nA workspace has at most one scope enabled at a time (set in Workspace Settings → MCP → Platform → Builder Mode). If a tool you expect isn't listed, the user picked the other scope or has Builder Mode off — surface that, don't guess.",
          "zh": "## propose → approve → apply 契约\n\n每个 Builder 动作都会展开成一对共享同一 payload schema 的工具：“<name>_propose” 和 “<name>_apply”。\n\n1. **Propose。** 你用设计好的 payload 调用 “<name>_propose”。它写入一个 pending 的 “agent_request”，并返回 { request_id, kind, label, payload, status: \"pending\" }。UI 用 payload 预览渲染审批卡。\n2. **用户决定。** 用户点击 Approve 或 Reject。Approve 时，聊天中会出现一条系统消息，指示你用 “request_id” 调用 “<name>_apply”。Reject 时，不发生任何事；不要重试，询问用户要改什么。\n3. **Apply。** 你调用 “<name>_apply(request_id)”。它重新解析 payload，验证 status 是 “approved”，通过 CAS 认领这个 request，然后执行效果。一次 approval = 一次 apply；并发尝试会在 CAS 上竞争，只有一个会成功。返回的文本就是用户在聊天中看到的内容。\n\n**绝不要伪造 “request_id”。** 只有当对话中明确携带 approval message 时才调用 apply；那是唯一合法的触发器。\n\n## 两种 scope\n\n| Scope | 工具名 | 触及内容 |\n|---|---|---|\n| “workspace”（默认） | “workspace_<resource>_propose” / “_apply” | 当前 workspace 自己的配置 |\n| “global” | “<resource>_propose” / “_apply”（无前缀） | 账号级资源；目前只有用户的 prompt library |\n\n一个 workspace 同一时间最多启用一个 scope（在 Workspace Settings → MCP → Platform → Builder Mode 中设置）。如果你预期的工具没有列出，说明用户选择了另一个 scope，或者关闭了 Builder Mode；把这一点告诉用户，不要猜。"
        },
        {
          "range": "L30-L77",
          "title": "读工具和通用规则",
          "original": "## Read tools are always on\n\nWhenever any Builder cap is enabled — regardless of scope — these read tools are available:\n\n- `list_prompts`, `get_prompt`\n- `list_skills`, `get_skill`\n- `list_providers`\n- `list_schedules`, `list_commands`, `get_command`\n- `get_workspace_config`\n- `list_sessions`, `get_session_export_urls` (past chats for analysis)\n\n**Always read before proposing.** Don't propose a schedule named \"Daily Review\" without checking if one already exists; don't propose switching the system prompt without first fetching the workspace's current prompt source. The read layer is what lets you ground proposals in reality.\n\n## Cross-cutting rules\n\n### Ownership\nPrompts in the library are owned by the user who created them. `prompt_update` / `prompt_delete` only succeed on prompts you own — team-shared and public prompts that show up in `list_prompts` are read-only from these tools.\n\n### Versioning\nOn `prompt_update`, a `content` change bumps the prompt's version; name / visibility-only changes do not. Workspaces pinned to that prompt pick up the new content on their next reload.\n\n### Reference safety\nYou cannot delete a prompt that any workspace still references. Apply hard-fails and lists the referring workspaces. The user must redirect those workspaces (via `workspace_prompt_propose` to a different prompt or inline text) before deletion can succeed.\n\n### Source XOR — silent-clobber trap\nThree resources share the same XOR pattern between inline content and a library reference:\n\n- **Workspace system prompt**: `system_prompt` (inline) vs `prompt_id` (library)\n- **Schedule**: `prompt` (inline) vs `prompt_id` (library)\n- **Slash command**: `prompt` (inline; stored as `content`) vs `prompt_id` (library)\n\nSetting one side automatically clears the other; never both. This is the trap when updating: if `get_command` / `list_schedules` / `get_workspace_config` shows a `library: <id>` marker (or the equivalent for prompts) but you propose an update with inline text, **the library reference is silently lost**. To preserve the reference, pass `prompt_id` instead; to switch to inline, pass `prompt` / `system_prompt`. Pass `\"\"` to clear either side explicitly.\n\n### Schedule timezones\nCron timezone is **required** — there is no silent UTC fallback. If the user says \"every morning at 9\" without naming a zone, ask before proposing. Use IANA names like `Asia/Shanghai` or `America/New_York`.\n\n### Visibility default\nNew prompts default to `private`. Pass `visibility: 'public'` only when the user has explicitly asked to share account-wide.\n\n### Provider / prompt visibility\nApply verifies that any `provider_id` or `prompt_id` is visible to the user. A propose with an unreachable id succeeds cheaply, but apply will fail. Prefer to pick from `list_providers` / `list_prompts` rather than passing a guessed id.\n\n## Flow tips\n\n- **One proposal at a time.** Don't queue multiple proposes hoping the user approves in order — each approval drives its own apply.\n- **Ask when ambiguous.** Timezone, visibility, delete impact, multi-step intent — clarify before proposing, not after.\n- **You design the diff.** When the user describes \"what I want\" rather than specific fields, your job is to translate that into a concrete payload. Read tools first, then propose.\n- **Apply result = user-facing text.** The string `_apply` returns is what the user reads in the chat — keep it factual and brief, no marketing.",
          "zh": "## 读工具始终开启\n\n只要启用了任意 Builder cap，不论 scope 是什么，以下读工具都可用：\n\n- “list_prompts”、“get_prompt”\n- “list_skills”、“get_skill”\n- “list_providers”\n- “list_schedules”、“list_commands”、“get_command”\n- “get_workspace_config”\n- “list_sessions”、“get_session_export_urls”（用于分析过去聊天）\n\n**提出 proposal 前始终先读取。** 不要在未检查是否已有同名 schedule 的情况下提出名为 “Daily Review” 的 schedule；不要在未先获取 workspace 当前 prompt source 的情况下提出切换 system prompt。读层让你的 proposal 能基于现实。\n\n## 横切规则\n\n### Ownership\nlibrary 中的 prompts 归创建它们的用户所有。“prompt_update” / “prompt_delete” 只会在你拥有的 prompts 上成功；team-shared 和 public prompts 即使出现在 “list_prompts” 中，对这些工具来说也是只读的。\n\n### Versioning\n在 “prompt_update” 上，“content” 变更会提升 prompt 版本；仅 name / visibility 变更不会。固定到该 prompt 的 workspaces 会在下一次 reload 时获得新内容。\n\n### Reference safety\n不能删除仍被任何 workspace 引用的 prompt。Apply 会硬失败并列出引用它的 workspaces。用户必须先把这些 workspaces 重定向（通过 “workspace_prompt_propose” 指向不同 prompt 或 inline text），删除才能成功。\n\n### Source XOR — 静默覆盖陷阱\n三个资源在 inline content 和 library reference 之间共享相同的 XOR 模式：\n\n- **Workspace system prompt**：“system_prompt”（inline） vs “prompt_id”（library）\n- **Schedule**：“prompt”（inline） vs “prompt_id”（library）\n- **Slash command**：“prompt”（inline；存储为 “content”） vs “prompt_id”（library）\n\n设置其中一侧会自动清空另一侧；永远不要两侧都设置。这是更新时的陷阱：如果 “get_command” / “list_schedules” / “get_workspace_config” 显示 “library: <id>” 标记（或 prompts 的等价标记），但你提出带 inline text 的更新，**library reference 会静默丢失**。要保留 reference，就传 “prompt_id”；要切换到 inline，就传 “prompt” / “system_prompt”。传 “” 可以显式清空任一侧。\n\n### Schedule timezones\nCron timezone 是**必需的**；没有静默 UTC fallback。如果用户说 “每天早上 9 点” 但没有说明时区，提出 proposal 前先询问。使用 “Asia/Shanghai” 或 “America/New_York” 这样的 IANA 名称。\n\n### Visibility default\n新 prompts 默认是 “private”。只有当用户明确要求账号范围共享时，才传 “visibility: public”。\n\n### Provider / prompt visibility\nApply 会验证任何 “provider_id” 或 “prompt_id” 对用户可见。使用不可达 id 的 propose 会低成本成功，但 apply 会失败。优先从 “list_providers” / “list_prompts” 中选择，而不是传猜测的 id。\n\n## 流程提示\n\n- **一次一个 proposal。** 不要排队多个 proposes 并希望用户按顺序批准；每次 approval 都驱动它自己的 apply。\n- **有歧义就询问。** Timezone、visibility、delete impact、多步骤意图；在 propose 前澄清，而不是之后。\n- **你设计 diff。** 当用户描述的是“我想要什么”而不是具体字段时，你的工作是把它翻译成具体 payload。先用读工具，再 propose。\n- **Apply result = 面向用户的文本。** “_apply” 返回的字符串就是用户在聊天中读到的内容；保持事实性和简短，不要营销。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/file-sharing.md",
      "title": "file-sharing.md",
      "sections": [
        {
          "range": "L1-L7",
          "title": "共享目录规则",
          "original": "# File Sharing\n\nShared folders at `/mnt/afs/<name>` exchange files with other agents without inlining them in prompts. Use for file handoffs, or when the target agent should read directly.\n\nTools: `share_folder` (create + mount here), `grant_access` (mount read-only on a target; the mount is ready before the call returns, so `call_agent` can follow immediately), `unshare_from_all` (cleanup). `grant_access` is optional — skip it for self-parking files.\n\nDon't inline file contents in `call_agent` when a shared folder fits, and don't pass access keys or dir ids in messages — the platform handles mounting.",
          "zh": "# 文件共享\n\n“/mnt/afs/<name>” 下的共享文件夹可以与其他 agents 交换文件，而不用把文件内联进 prompts。用于文件交接，或者当目标 agent 应该直接读取时使用。\n\n工具：“share_folder”（创建并在这里挂载）、“grant_access”（在目标上以只读方式挂载；调用返回前 mount 已就绪，所以可以立即跟随 “call_agent”）、“unshare_from_all”（清理）。“grant_access” 是可选的；如果只是自己临时放文件，可以跳过。\n\n当共享文件夹适用时，不要在 “call_agent” 中内联文件内容，也不要在消息里传 access keys 或 dir ids；平台会处理挂载。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/image-generation.md",
      "title": "image-generation.md",
      "sections": [
        {
          "range": "L1-L3",
          "title": "Codex 图片保存位置",
          "original": "# Image Generation (Codex)\n\nGenerated images save to `/workspace/.home/.codex/generated_images/`. Surface them inline with `![generated](/workspace/.home/.codex/generated_images/<filename>)`.",
          "zh": "# 图片生成（Codex）\n\n生成的图片保存到 “/workspace/.home/.codex/generated_images/”。用 “![generated](/workspace/.home/.codex/generated_images/<filename>)” 将它们以内联方式展示。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/memory.md",
      "title": "memory.md",
      "sections": [
        {
          "range": "L1-L5",
          "title": "Memory store 定义",
          "original": "# Memory\n\nMemory stores are persistent text directories mounted into the workspace at `/mnt/memory/<store_id>/`. They survive across sessions, restarts, and pod rebuilds — anything written here is durable. Stores must be attached to the workspace via the NAP UI; an unattached workspace sees no `/mnt/memory/` entries and this capability is unavailable.\n\nThe platform reminder lists each attached store with its `(access)` and any workspace-specific instructions. `read_only` mounts reject writes at the filesystem level; do not retry on `EROFS` — surface the error to the user.",
          "zh": "# Memory\n\nMemory stores 是挂载到 workspace 中 “/mnt/memory/<store_id>/” 的持久文本目录。它们会跨会话、重启和 pod 重建保留；写到这里的任何内容都是持久的。Stores 必须通过 NAP UI attach 到 workspace；未 attach 的 workspace 看不到任何 “/mnt/memory/” 条目，此能力不可用。\n\nplatform reminder 会列出每个 attached store，包括它的 “(access)” 和任何 workspace 专属 instructions。“read_only” 挂载会在文件系统层拒绝写入；遇到 “EROFS” 不要重试，把错误展示给用户。"
        },
        {
          "range": "L7-L31",
          "title": "磁盘 schema",
          "original": "## On-disk schema\n\nEach memory is one file holding one fact. Use markdown with frontmatter:\n\n```markdown\n---\nname: <short-kebab-case-slug>\ndescription: <one-line summary — used to decide relevance during recall>\nmetadata:\n  type: user | feedback | project | reference\n  created: <YYYY-MM-DD>\n---\n\n<the fact; for feedback/project, follow with **Why:** and **How to apply:** lines.\nLink related memories with [[their-name]].>\n```\n\nType meanings:\n\n- `user` — who the user is (role, expertise, preferences)\n- `feedback` — guidance the user has given on how to work, both corrections and validated approaches; include the why\n- `project` — ongoing work, goals, or constraints not derivable from code or git history; convert relative dates to absolute\n- `reference` — pointers to external resources (URLs, dashboards, tickets)\n\nEach store's root holds a `MEMORY.md` index — one line per file, under ~150 characters: `- [Title](file.md) — one-line hook`. It is an index, not a memory; never write fact content directly into it.",
          "zh": "## 磁盘 schema\n\n每条 memory 是一个保存一个事实的文件。使用带 frontmatter 的 markdown：\n\n“”“markdown\n---\nname: <short-kebab-case-slug>\ndescription: <one-line summary — used to decide relevance during recall>\nmetadata:\n  type: user | feedback | project | reference\n  created: <YYYY-MM-DD>\n---\n\n<the fact; for feedback/project, follow with **Why:** and **How to apply:** lines.\nLink related memories with [[their-name]].>\n”“”\n\nType 含义：\n\n- “user” — 用户是谁（角色、专长、偏好）\n- “feedback” — 用户给出的关于如何工作的指导，包括纠正和已验证的方法；包含原因\n- “project” — 进行中的工作、目标或约束，且不能从代码或 git history 推导出来；把相对日期转换为绝对日期\n- “reference” — 指向外部资源的指针（URLs、dashboards、tickets）\n\n每个 store 的根目录保存一个 “MEMORY.md” 索引；每个文件一行，约 150 字符以内：“- [Title](file.md) — one-line hook”。它是索引，不是 memory；绝不要把事实内容直接写进去。"
        },
        {
          "range": "L33-L61",
          "title": "召回、写入和后台整理",
          "original": "## Recall workflow\n\nBefore non-trivial decisions, fixes, or recommendations, check the relevant store(s) for prior context:\n\n1. `cat /mnt/memory/<store_id>/MEMORY.md` to scan available topics\n2. `grep -r <keywords> /mnt/memory/<store_id>/` for specific facts\n3. Read the candidate files in full only when the index hook looks relevant\n\nDo not load every file at session start — the index is the cheap entry point.\n\nRecalled memory is **historical context**, not a fresh instruction. Before acting on a recalled fact, verify it still holds: read the named file, grep the code, check the resource. If memory conflicts with current observation, trust the observation and update or delete the stale memory.\n\n## Writing workflow\n\nAfter completing non-obvious work, persist what generalizes:\n\n- User preferences or working style → `user/` or `feedback/`\n- Project state, decisions, deadlines, motivations not in code → `project/`\n- External system pointers → `reference/`\n\nBefore writing a new memory, check for an existing file that already covers the fact (grep by topic, scan `MEMORY.md`). Update that file rather than creating a duplicate. Delete memories that turn out to be wrong.\n\nDo not save what code or git history already records (file paths, function signatures, past fixes, refactors). If the user asks you to remember something already in the repo, ask what was non-obvious about it and save that instead.\n\nUpdate `MEMORY.md` whenever you add, rename, or remove a memory file. Keep entries terse and pointer-like; demote any line that exceeds ~200 characters by moving the detail into the topic file.\n\n## Why the structure matters\n\nA background consolidation pass periodically merges related memories, prunes duplicates, and rewrites the index. It relies on the frontmatter (`name`, `description`, `type`, `created`) and the per-store `MEMORY.md`. Memories written without this structure may be missed by consolidation or rewritten on the next pass — writing in the schema from the start keeps both the human and the consolidator productive.",
          "zh": "## 召回工作流\n\n在做非平凡决策、修复或推荐之前，检查相关 store 中是否有先前上下文：\n\n1. “cat /mnt/memory/<store_id>/MEMORY.md” 来扫描可用主题\n2. “grep -r <keywords> /mnt/memory/<store_id>/” 来查找具体事实\n3. 只有当 index hook 看起来相关时，才完整读取候选文件\n\n不要在会话开始时加载每个文件；索引是低成本入口。\n\n召回的 memory 是**历史上下文**，不是新的指令。根据召回事实行动前，要验证它仍然成立：读取具名文件，grep 代码，检查资源。如果 memory 与当前观察冲突，相信当前观察，并更新或删除过时 memory。\n\n## 写入工作流\n\n完成非显而易见的工作后，持久化可泛化的内容：\n\n- 用户偏好或工作风格 → “user/” 或 “feedback/”\n- 不在代码中的项目状态、决策、截止日期、动机 → “project/”\n- 外部系统指针 → “reference/”\n\n写入新 memory 前，检查是否已有文件覆盖该事实（按主题 grep，扫描 “MEMORY.md”）。更新已有文件，而不是创建重复项。删除后来证明错误的 memories。\n\n不要保存代码或 git history 已经记录的内容（文件路径、函数签名、过去的修复、重构）。如果用户要求你记住 repo 中已有的东西，询问其中非显而易见的部分是什么，并保存那个内容。\n\n每当你添加、重命名或删除 memory 文件时，都更新 “MEMORY.md”。保持条目简洁、像指针；任何超过约 200 字符的行，都应把细节移到主题文件中。\n\n## 为什么结构很重要\n\n后台整理流程会定期合并相关 memories、修剪重复项并重写索引。它依赖 frontmatter（“name”、“description”、“type”、“created”）和每个 store 的 “MEMORY.md”。没有按此结构写入的 memories 可能会被整理流程漏掉，或在下一轮被重写；从一开始就按 schema 写，可以让人和整理器都保持高效。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/sandbox.md",
      "title": "sandbox.md",
      "sections": [
        {
          "range": "L1-L14",
          "title": "沙箱使用和 cwd",
          "original": "# Sandbox\n\nUse sandbox MCP tools for untrusted code, unavailable runtimes, or anything that would pollute the workspace. To preview a web app: `create_sandbox` → start dev server → `sandbox_get_preview_url`. (Browser is for *interacting* with web pages, not previewing your own.)\n\n## Default working directory\n\nThe sandbox container starts with cwd `/` (root). There is no pre-created `/workspace` directory. When running commands that produce or consume files, always pass an explicit `cwd` or create the directory first:\n\n```\nsandbox_run_command: mkdir -p /home/user && ...\nsandbox_run_command: <your command>, cwd: \"/home/user\"\n```\n\nUse `/tmp` for ephemeral scratch files and `/home/user` (or any path you `mkdir`) for project work.",
          "zh": "# 沙箱\n\n对不受信任的代码、不可用的运行时，或者任何会污染 workspace 的内容，使用 sandbox MCP 工具。预览 web app： “create_sandbox” → 启动 dev server → “sandbox_get_preview_url”。（Browser 用于与网页交互，不用于预览你自己的网页。）\n\n## 默认工作目录\n\nsandbox 容器启动时 cwd 是 “/”（root）。没有预先创建的 “/workspace” 目录。运行会生成或消费文件的命令时，始终传显式 “cwd”，或先创建目录：\n\n“”“\nsandbox_run_command: mkdir -p /home/user && ...\nsandbox_run_command: <your command>, cwd: \"/home/user\"\n”“”\n\n使用 “/tmp” 存放临时草稿文件，使用 “/home/user”（或任何你 “mkdir” 的路径）存放项目工作。"
        }
      ]
    },
    {
      "path": "agent-skills/platform/__platform__/reference/skills.md",
      "title": "skills.md",
      "sections": [
        {
          "range": "L1-L3",
          "title": "Skill 生命周期工具",
          "original": "# Skills\n\nTo create or edit a skill, use `skill_create_draft` / `skill_enter_edit` / `skill_publish`. Follow the `skill-creator` skill for SKILL.md conventions (ask the user to install it if missing).",
          "zh": "# Skills\n\n要创建或编辑 skill，使用 “skill_create_draft” / “skill_enter_edit” / “skill_publish”。遵循 “skill-creator” skill 的 SKILL.md 约定（如果缺失，请让用户安装它）。"
        }
      ]
    }
  ]
};
