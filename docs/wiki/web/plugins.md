# plugins.md - 插件系统

> 位于 `web/src/plugins/`（内置）+ `web/src/lib/plugin-loader.ts`（外部加载器）+ `web/vite.config.ts`（dev 中间件）。

---

## 1. 为什么要有插件系统

平台核心交互是「workspace 里 agent 调用工具 → 工具结果回流 UI」。有三类需求不该塞进主应用代码：

1. **自定义面板** - 某团队想在 workspace 加一个「README 解释器」按钮面板，点击即往聊天发预设 prompt
2. **自定义工具渲染卡片** - `workspace_schedule_propose` 这类工具结果要渲染成带 Approve/Reject 的交互卡，而非纯文本
3. **工具完成后的副作用** - agent 调了 Edit/Write，文件树面板需自动刷新

底层逻辑：把 UI 扩展能力从「编译期依赖」降级成「运行期 manifest 驱动」，让第三方能独立交付插件而不动主干。平台只负责加载 IIFE bundle + 暴露 `window.tos` 宿主 API；插件自己调 `registerPanel/registerToolRenderer/registerToolHandler` 完成注册。和 Django app config / VS Code extension 的分工一致。

> 关键不变量：**平台只负责加载脚本 + 提供 `window.tos`**；**插件 IIFE 脚本负责调 `registerXxx`**。

## 2. 插件能扩展的三种 surface

| surface | 注册 API | 触发点 |
|---|---|---|
| panels | `registerPanel()` | workspace 视图渲染、用户点 tab |
| toolRenderers | `registerToolRenderer()` | 聊天流遇到匹配 toolName 的 tool_call |
| toolResultHandlers | `registerToolHandler()` | SSE 流遇到 tool_result 事件匹配 pattern |

## 3. 现有插件

### 内置插件（`web/src/plugins/`，编译进主 bundle，boot 时 `registerBuiltinPlugins()` 同步注册）

| 插件 | 扩展的 surface | 职责 |
|---|---|---|
| filesPlugin | toolResultHandler | 匹配 Edit/Write/apply_patch 等，bump 文件面板刷新信号（带防抖，agent 常连发 Write→Edit→Edit） |
| memoryPlugin | toolResultHandler | 占位注册，转发 `memory.invalidate` 事件给面板 |
| skillsPlugin | toolResultHandler | 匹配技能管理工具，刷新技能目录 |
| builderModePlugin | toolRenderer + toolResultHandler | 渲染 `*_propose` 类工具的 Approve/Reject 卡；`*_apply` 完成后刷新对应资源 |

### 内置 Tool Renderers

Agent Request Propose 类工具的渲染器（交互式 Approve/Reject 卡片）：

- `workspace_schedule_propose` / `workspace_schedule_update_propose` / `workspace_schedule_delete_propose`
- `workspace_command_propose` / `workspace_command_update_propose` / `workspace_command_delete_propose`
- `workspace_skill_enable_propose` / `workspace_skill_disable_propose`
- `workspace_config_propose` / `workspace_prompt_propose`
- `prompt_create_propose` / `prompt_update_propose` / `prompt_delete_propose`

### 外部插件

第三方写的 IIFE bundle，通过 admin 上传到 control-plane DB（`control-plane/src/routes/plugins.ts:64`，multipart 上传 bundle + metadata JSON，限 20MB），DB `enabled` 字段控制开关。

## 4. 插件契约（types.ts）

### ToolMatchCtx

SSE 流中 tool_result 事件的匹配上下文：

| 字段 | 说明 |
|---|---|
| toolName | 工具名（如 "Edit", "mcp__tos-platform__share_folder"） |
| toolInput | 工具输入参数 |
| toolOutput | 工具输出 |
| isError | 是否出错 |
| callId | 调用 ID |
| workspaceId | 所属 workspace |

### ToolResultHandler

| 字段 | 说明 |
|---|---|
| id | 稳定标识（惯例：`<plugin>.<purpose>`） |
| match(ctx) | 匹配函数 |
| debounceMs | 防抖毫秒数 |
| onMatch(ctx) | 匹配回调 |

### WorkspacePlugin

| 字段 | 说明 |
|---|---|
| id | 插件 ID |
| toolResultHandlers? | 工具结果处理器列表 |
| panels? | 贡献到 workspace 视图的面板列表 |

> panel `id` 必须与某条 `mcp_catalog` 条目的 `ui_panel` 值匹配，tab 才会显示。

## 5. 注册中心（registry.ts）

模块级单例，管理所有已注册的 ToolResultHandler。

### createToolResultDispatcher()

每个 AgentSessionStore 创建一个 dispatcher：

1. `dispatch(ctx)` - 遍历所有 handler，匹配则触发
2. 支持防抖（同一 handler id 短时间内只触发一次）
3. 匹配懒加载插件时触发 `ensurePluginLoaded()`
4. `destroy()` - 清理所有定时器

## 6. 加载机制

### 两条 manifest 路径（dev / prod）

| 维度 | Dev | Prod |
|---|---|---|
| manifest 端点 | `/dev-plugins/manifest.json`（vite 中间件扫盘） | `/api/plugins`（cp 从 DB 读 enabled=true 行） |
| bundle URL | `/dev-plugins/<id>/<id>.js` | `/api/plugins/<id>/bundle.js?v=<version>` |
| bundle 存储 | 文件系统（`TOS_DEV_PLUGINS_DIR` 下） | control-plane DB blob |
| 缓存策略 | `no-cache`（每次请求重扫盘） | `public, max-age=31536000, immutable` + ETag + 版本化 URL |
| 上传方式 | 文件落地即可 | admin multipart POST，校验 id/版本正则 + 20MB 上限 |
| 启用控制 | 文件存在即生效 | DB `enabled` 字段，admin PATCH 开关 |
| 触发条件 | `import.meta.env.DEV` 才请求 | 任何环境，boot 时拉 |

**覆盖语义**：dev manifest 按 `id` 覆盖 cp 发布的 manifest，本地版顶掉线上版，不影响其他线上插件。

### boot 时序（`main.tsx`）

1. `installHost()` - 挂 `window.tos` 宿主 API（registerPanel/registerToolRenderer/registerToolHandler 等）
2. `registerBuiltinPlugins()` - 内置插件同步注册
3. `loadExternalPlugins()` - fetch `/api/plugins` + `fetchDevManifest()`（DEV only，按 id 覆盖），合并后逐条加载
4. `createRoot().render()` - 渲染 React

### 每条 manifest 的两种加载模式

- **eager**（默认，`lazy` 缺省时）：boot 时一次性 `<script>` 注入 IIFE bundle，插件立即跑 `window.tos.registerXxx`。向后兼容老插件。

- **lazy**（`metadata.lazy=true` 且声明 `owns`）：boot 时只注册占位描述符（panel 的静态 label、toolRenderer 的 toolName、handler 的 pattern）。三种 surface 各自独立触发 `ensurePluginLoaded(pluginId)`：
  - 用户点 panel tab → 注入 bundle
  - 聊天渲染匹配 toolRenderer 的 tool_call → 注入 bundle
  - tool_result 事件匹配 handler pattern → 注入 bundle

  `owns` 字段（`plugins.ts:30-32`）声明该插件占哪些 surface，让 host 知道何时该懒加载它。顶层设计上 lazy 是为了不拖慢首屏——一个 workspace 可能挂了一堆面板插件，但用户本次会话只用一个，没必要全量注入。

### 失败处理

`fetchManifest()` 对状态码做了区分（`plugin-loader.ts:53`）：

- `404` → endpoint/manifest 真不存在，返回 `[]`（终态，不重试）
- `401/403/5xx` → 可重试失败，返回 `null`。`null`（vs `[]`）让 `loadExternalPlugins` 不缓存空结果，等用户认证后重试——否则插件 app 会一直缺失直到整页刷新。

## 7. Dev 模式的意义

外部插件开发闭环本来是「改代码 → build → 上传 cp → 刷新页面」。dev 环境往往没有 admin token 上传，而且每次改一行都要重传 20MB bundle，反馈闭环是分钟级。

dev-plugins 中间件（`web/vite.config.ts:15`）做的事：设 `TOS_DEV_PLUGINS_DIR` 指向插件源码目录后，vite dev server 每次 HTTP 请求都重扫盘，把 `<id>/dist/<id>.js` 临时拼成 manifest 返回。配合插件目录里 `vite build --watch`，**改一行 → 重新 build → 刷新页面**就能看到效果，零上传、零 token、零重启。

约定的磁盘结构（`vite.config.ts:32`）：

```
$TOS_DEV_PLUGINS_DIR/
├── <plugin-id>/
│   └── dist/
│       └── <plugin-id>.js     # bundle 入口，文件名必须等于目录名
└── another-plugin/
    └── dist/
        └── another-plugin.js
```

中间件还挂了 `/dev-plugins/` 静态文件路由（`vite.config.ts:50`），把 `/dev-plugins/<id>/dist/<id>.js` 映射回磁盘，带 `file.startsWith(absRoot)` 防路径穿越。

> 抓手就是「把上传闭环换成文件系统闭环」，颗粒度细到单次 HTTP 请求重扫盘。

## 8. 代码注释

### plugins/builder-mode/index.ts

- `plugins/builder-mode/index.ts:1` - Builder-mode plugin - owns refresh signals for the resources that the agent can mutate through `*_apply` tools....
- `plugins/builder-mode/index.ts:33` - One token per panel-scoped data domain. Add a new entry here when wiring a new builder-mode action whose UI lives outside the existing panels.
- `plugins/builder-mode/index.ts:40` - MCP-namespaced names arrive as e.g. `mcp__tos-platform__workspace_schedule_apply`. Anchor on word boundaries so partial matches don't slip through.

### plugins/files/index.ts

- `plugins/files/index.ts:1` - Files plugin - owns the "workspace files have changed" refresh signal.  Tools the agent commonly uses to mutate files (Edit/Write/apply_patch,...
- `plugins/files/index.ts:31` - Agents often produce Write -> Edit -> Edit in quick succession; coalesce.
- `plugins/files/index.ts:34` - Match when the tool name *contains* one of these identifiers as a word-boundary segment. Used for MCP-namespaced tools where the name arrives as e.g....
- `plugins/files/index.ts:40` - Match when the tool name *starts with* one of these (Claude emits the bare name "Edit"; Codex prefixes "Edit /path", "Run pwd")....

### plugins/index.ts

- `plugins/index.ts:1` - Plugin bootstrap - registered once at app startup.  Each plugin owns its own refresh signal and exports a public bump/useToken API; the agent-session...
- `plugins/index.ts:17` - Builder Mode propose tools render an interactive Approve/Reject card that reaches into app state,...

### plugins/memory/index.ts

- `plugins/memory/index.ts:1` - Memory plugin - placeholder registration so external plugins can fan `memory.invalidate` events through the host bus and the panel can react via...

### plugins/registry.ts

- `plugins/registry.ts:1` - Plugin registry - module-level singleton.  Plugins register at app startup; the agent-session store creates a per-store dispatcher whose lifetime...
- `plugins/registry.ts:31` - Test helper - clears all registered handlers. export function _resetPluginsForTests(): void { handlers.length = 0 seenHandlerIds.clear() } export...

### plugins/skills/index.ts

- `plugins/skills/index.ts:1` - Skills plugin - owns the "skills directory has changed" refresh signal.  Bumps when the agent calls one of the skill-management MCP tools...
- `plugins/skills/index.ts:31` - Builder-mode workspace_skill_(enable|disable)_apply changes which skills are attached to the workspace -...

### plugins/types.ts

- `plugins/types.ts:1` - Workspace plugin contract.  Plugins extend the workspace with side-effects on tool completions....
- `plugins/types.ts:14` - Tool name as it arrives on the SSE stream - may be bare ("Edit"),  prefixed with an argument ("Edit /path",...
- `plugins/types.ts:26` - Stable id for debugging / debounce keying. Convention: `<plugin>.<purpose>`. id: string match: (ctx: ToolMatchCtx) => boolean Coalesce rapid matches...
- `plugins/types.ts:39` - Tabs the plugin contributes to the workspace view. The panel `id` must  match the `ui_panel` value of an mcp_catalog entry; the tab only shows  when...
