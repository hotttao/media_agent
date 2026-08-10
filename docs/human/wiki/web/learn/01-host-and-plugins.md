# 01 主机层与插件三件套

> 分章。讲 `window.tos` 这个全局对象的设计、三个注册函数、内置与外部插件的区别、eager/lazy 加载机制、工具结果调度器。读完你应该能写出一个最小可用的外部插件。

## 代码地图

| 文件 | 行数 | 职责 |
|------|------|------|
| `lib/host.ts` | 109 | 定义 `TosHost` 接口 + `installHost()` 装到 `window.tos` |
| `lib/host-events.ts` | ~30 | 事件总线 `events.on/emit` |
| `lib/panel-registry.ts` | 54 | 面板注册表：`Map<id, PluginPanel>` + 订阅 |
| `lib/plugin-lazy-registry.ts` | ~70 | 延迟插件的占位描述符表（panel/renderer/handler 三类） |
| `lib/plugin-loader.ts` | 204 | 拉 `/api/plugins` 清单 + 注入 script + lazy 按需加载 |
| `plugins/registry.ts` | 73 | `registerPlugin()` + `createToolResultDispatcher()` |
| `plugins/types.ts` | ~50 | `WorkspacePlugin` / `ToolResultHandler` / `ToolMatchCtx` 类型 |
| `plugins/index.ts` | 51 | `registerBuiltinPlugins()` 注册内置插件 + 13 个工具渲染器 |

外部 SDK 包（不在本仓库）：

- `@neutree-ai/ui-sdk` - 工具渲染器注册表实际存放处
- `@neutree-ai/theme` - `useResolvedTheme`
- `@neutree-ai/sse-consumer` - SSE 流消费（见 [03 章](./03-shell-and-chat.md)）

## 抓手拆解

### 抓手 1：`window.tos` 是什么

`lib/host.ts:1-14` 的注释把设计意图说得很清楚：

> Plugin host - the single global surface (`window.tos`) that external plugins consume at runtime. Plugins are IIFE bundles loaded after this module has run; **sharing React from here is mandatory** (the rules of hooks require a single React instance).
>
> Surface is intentionally minimal. New entries land here only when a real plugin needs a piece of host state or logic that it cannot reasonably reimplement.

两个关键约束：

1. **必须共享 React 实例**。React hooks 规定全局只能有一个 React 实例，外部插件不能自己 `import React`，必须从 `window.tos.React` 拿。同理 `ReactDOM` 和 `ReactJSXRuntime`（JSX 运行时）。
2. **表面刻意最小**。展示层库（shadcn/Radix/lucide/zustand/i18next）和领域 API 不进主机，插件自己解决。主机只放"无法合理重实现的 nap 专属行为"（agent session 状态、workspace file overlay、带文件链接的 markdown 渲染器）。

### 抓手 2：`TosHost` 的命名空间

`lib/host.ts:41-73` 定义接口，按命名空间分组：

| 命名空间 | 成员 | 来源 |
|----------|------|------|
| 顶层 | `version` `React` `ReactDOM` `ReactJSXRuntime` `events` | React 单例 + 事件总线 |
| 顶层（注册三件套） | `registerPlugin` `registerPanel` `registerToolRenderer` | 转发到各 registry |
| `ui` | `Markdown` `useResolvedTheme` | 自有 Markdown 渲染器 + 主题 hook |
| `workspace` | `useSlotContext` `useAgentSessionActions` `useAgentSessionStore` `useSessionNavigation` `useInstanceState` `useInstancePersistentState` | 6 个会话/slot/实例状态 hook |
| `windows` | `useAppHeaderSlot` `AppHeaderButton` | 窗口标题栏定制 |
| `tools` | `getMcpText` `unwrapMcpInput` `safeParseResult` `truncate` `jsonPreview` | MCP 工具输出解析辅助 |

**记忆口诀**：顶层是 React 共享 + 注册三件套 + 事件总线；`ui` 给 UI 原语；`workspace` 给状态 hooks；`windows` 给窗口装饰；`tools` 给 MCP 工具解析。

### 抓手 3：三个注册函数的差异

这是本章核心。三个 `registerXxx` 对应三种独立的扩展点，存放在三个独立的注册表里：

| 注册函数 | 签名 | 存在哪 | 注册的是什么 | 谁会调用 |
|----------|------|--------|-------------|---------|
| `registerPlugin` | `(plugin: WorkspacePlugin) => void` | `plugins/registry.ts:17` | 一个插件包（含 toolResultHandlers + panels） | 内置插件 / 外部插件 script |
| `registerPanel` | `(panel: PluginPanel) => void` | `lib/panel-registry.ts:30` | 一个面板（id + label + component） | `registerPlugin` 内部循环 / 外部 script 直接调 |
| `registerToolRenderer` | `(name, Component)` | `@neutree-ai/ui-sdk` 内部 | 一个工具名 -> 渲染组件的映射 | `plugins/index.ts:47` 内置 / 外部 script |

**关键差异**：

- `registerPlugin` 是**打包式注册**：一个插件可以一次带上多个 handler 和多个 panel。
- `registerPanel` 是**原子注册**：单个面板进 `Map`，重复 id 会被警告并跳过（`lib/panel-registry.ts:31-34`）。
- `registerToolRenderer` **不在本仓库的注册表里**，而是 re-export 自 `@neutree-ai/ui-sdk`（`lib/host.ts:89`）。SDK 内部维护。原因是工具渲染器只在聊天 transcript 渲染时用，SDK 才是渲染方。

### 抓手 4：`WorkspacePlugin` 形状

`plugins/types.ts:36-43`：

```typescript
interface WorkspacePlugin {
  id: string
  toolResultHandlers?: ToolResultHandler[]   // 工具结果匹配 + 副作用
  panels?: PluginPanel[]                      // 工作区面板贡献
}
```

`ToolResultHandler`（`plugins/types.ts:25-32`）：

```typescript
interface ToolResultHandler {
  id: string                    // 约定: <plugin>.<purpose>
  match: (ctx: ToolMatchCtx) => boolean
  debounceMs?: number           // 去抖窗口
  onMatch: (ctx: ToolMatchCtx) => void
}
```

`ToolMatchCtx`（`plugins/types.ts:13-23`）：

```typescript
interface ToolMatchCtx {
  toolName: string
  toolInput: Record<string, unknown>
  toolOutput: string
  isError: boolean
  callId: string
  workspaceId: string
}
```

**设计要点**：plugin 不持有生命周期钩子（mount/unmount），只有"工具结果事件匹配 + 副作用"这一种扩展点。生命周期由 store 隐式管理 - store 销毁时调 `dispatcher.destroy()` 清掉所有去抖定时器（`plugins/registry.ts:66-69`）。

### 抓手 5：`PluginPanel` 形状

`lib/panel-registry.ts:8-17`：

```typescript
interface PluginPanel {
  /** Matches `ui_panel` on mcp_catalog entries; the workspace shows the panel
   *  iff an enabled MCP server's catalog row points to this id. */
  id: string
  /** Returns the localized tab label. Called every render so plugins with
   *  their own i18next instance can react to language changes. */
  label: () => string
  component: ComponentType<PluginPanelProps>
}

interface PluginPanelProps {
  workspaceId: string
  instanceId: string
}
```

**关键设计**：

1. **面板靠 id 匹配 MCP 目录**。面板不是直接挂到 slot 上，而是通过 MCP catalog 里的 `ui_panel` 字段间接关联。只有当某个启用的 MCP 服务器 catalog 行指向这个 id 时，面板才出现在工作区里。这是插件与 MCP 服务器的耦合点。
2. **`label` 是函数不是字符串**。每次渲染调用，支持动态 i18n。主机在 `lib/host.ts:107` 把 i18next 的 `languageChanged` 事件桥到 `events` 总线的 `lang.change`，插件用自己的 i18next 实例监听这个事件重渲染。
3. **`component` 拿到 `workspaceId` + `instanceId`**。instanceId 是 Slot 模型里的实例 id（见 [03 章](./03-shell-and-chat.md)），让面板能隔离自己的状态。

### 抓手 6：内置 vs 外部插件

| 维度 | 内置插件 | 外部插件 |
|------|---------|---------|
| 代码位置 | `plugins/files/` `plugins/skills/` `plugins/builder-mode/` `plugins/memory/` | 独立仓库，构建为 IIFE bundle |
| 注册方式 | `registerBuiltinPlugins()` 在 `main.tsx:31` 同步调用 | `loadExternalPlugins()` 注入 `<script>` 标签异步加载 |
| 是否经过 `window.tos` | 否（直接 import） | 是（必须，因为不能重复 import React） |
| 工具渲染器 | `plugins/index.ts:46-48` 直接调 `registerToolRenderer` | 在 script 里调 `window.tos.registerToolRenderer` |
| 当前内置 | 4 个：files、memory、skills、builderMode | 由 `/api/plugins` 清单动态决定 |

`registerBuiltinPlugins` 做的事（`plugins/index.ts:39-51`）：

```typescript
registerPlugin(filesPlugin)        // 文件变更 -> invalidateQueries
registerPlugin(memoryPlugin)       // 占位（无 handler、无 panel）
registerPlugin(skillsPlugin)       // 技能目录变更 -> invalidateQueries
registerPlugin(builderModePlugin)  // plan/command/config/prompt 变更 -> invalidateQueries
// 13 个 *_propose 工具的渲染器
for (const name of AGENT_REQUEST_PROPOSE_TOOLS) {
  registerToolRenderer(name, agentRequestProposeRenderer)
}
// call_agent 结果里的 session id 变成跳转链接
setSubAgentSessionLink(SubAgentSessionLink)
```

注意 `AGENT_REQUEST_PROPOSE_TOOLS`（`plugins/index.ts:21-35`）是 13 个 `*_propose` 工具名，全部挂同一个渲染器 `agentRequestProposeRenderer`（交互式 Approve/Reject 卡片，因为要触达 app 状态所以注册在 app 侧而不是 SDK 里）。

### 抓手 7：eager vs lazy 加载

`lib/plugin-loader.ts` 是外部插件加载的核心。两种模式由清单条目的 `lazy` 字段决定：

**eager（默认）**（`plugin-loader.ts:160-173`）：

- boot 时按顺序注入 `<script src={bundleUrl} async={false}>`。
- script 执行完立即调用 `window.tos.registerXxx`，注册表立即更新。
- `loaded` 集合标记防重复注入。

**lazy**（`plugin-loader.ts:178-203` + `registerLazyDescriptors`）：

- boot 时**不**注入 script。
- 调用 `registerLazyDescriptors(entry)` 把插件声明的 surfaces 注册成**占位描述符**：
  - `owns.panels` -> `registerLazyPanel(id, {pluginId, label})`（在 `lib/plugin-lazy-registry.ts`）
  - `owns.toolRenderers` -> `registerLazyToolRenderer(toolName, pluginId)`
  - `owns.toolHandlers` -> `registerLazyToolHandler(RegExp(pattern), pluginId)`
- 用户与任何占位 surface 交互时，触发 `ensurePluginLoaded(pluginId)`（`plugin-loader.ts:105-127`）真正注入 script。

**lazy 的三种触发路径**：

1. **面板点击** - 用户点开 lazy 插件面板 -> `useWsApps` 发现是 lazy -> `ensurePluginLoaded`
2. **工具渲染匹配** - 聊天 transcript 里出现该插件声明的 toolName -> SDK 通过 `LazyToolRenderers` 桥（见 `TranscriptProviders.tsx:41`）-> `ensurePluginLoaded`
3. **工具结果匹配** - SSE 流来 `tool_result` 事件 -> `createToolResultDispatcher` 调 `matchLazyToolHandlers(toolName)`（`plugins/registry.ts:45-47`）-> `ensurePluginLoaded`

**`ensurePluginLoaded` 的去重**（`plugin-loader.ts:105-127`）：

- `loaded` Set：已加载完成 -> 直接 resolve
- `pendingLoads` Map：正在加载 -> 共享同一个 Promise
- 找不到 URL（不是 lazy 条目）-> 直接 resolve
- 否则 `injectScript(url)` -> 成功后入 `loaded`，失败抛错

**注意 lazy 的"丢失当前事件"设计**（`plugins/registry.ts:41-47` 注释）：

> Lazy plugins: a matching tool name triggers a load, but the current event is dropped - the bundle's `registerPlugin` will pick up subsequent matching events. Accepted miss.

即首次匹配只触发加载，不执行 handler。bundle 加载完才会注册真正的 handler，后续事件才能命中。这是有意的取舍：避免阻塞 SSE 流等 bundle 下载。

### 抓手 8：清单获取与重试

`fetchManifest()`（`plugin-loader.ts:53-69`）拉 `/api/plugins`：

- 200 + 数组 -> 返回数组（可能空）
- 404 -> 返回 `[]`（端点根本不存在，最终结果）
- 401/403/5xx -> 返回 `null`（可重试失败，**不**缓存空）

`fetchDevManifest()`（`plugin-loader.ts:76-86`）只读 dev 环境 `/dev-plugins/manifest.json`，由 Vite dev server 从 `TOS_DEV_PLUGINS_DIR` 扫盘。dev 条目按 id 覆盖/扩展发布清单。

`loadExternalPlugins()`（`plugin-loader.ts:139-146`）的三层守卫：

- `manifestLoaded` - 成功后置 true，AuthProvider 后续再调变 no-op
- `inflight` - boot 与 auth effect 竞态时共享一个 Promise
- `doLoadExternalPlugins` 里如果 `published === null && dev.length === 0`，**直接 return 不置 `manifestLoaded`**（`plugin-loader.ts:153`），让登录后重试

**这个 return 是修过 bug 的痕迹**：注释说"Caching an empty list here is exactly the bug that hid plugin apps until reload"。读代码时看到这种 return + 注释，说明这块的状态机很微妙，改之前要小心。

### 抓手 9：工具结果调度器

`createToolResultDispatcher()`（`plugins/registry.ts:37-72`）是插件能力的运行期入口：

```typescript
function dispatch(ctx: ToolMatchCtx): void {
  // 1. 先查 lazy 注册表，触发可能的外部插件加载
  for (const pluginId of matchLazyToolHandlers(ctx.toolName)) {
    ensurePluginLoaded(pluginId).catch(() => {})
  }
  // 2. 遍历已注册的 handler，逐个 match
  for (const h of handlers) {
    if (!h.match(ctx)) continue
    if (h.debounceMs) {
      // 去抖：相同 handler id 的旧 timer 清掉，新 timer 排上
      const existing = timers.get(h.id)
      if (existing) clearTimeout(existing)
      timers.set(h.id, setTimeout(() => {
        timers.delete(h.id)
        h.onMatch(ctx)
      }, h.debounceMs))
    } else {
      h.onMatch(ctx)
    }
  }
}
```

**两个关键设计**：

1. **每个 handler 独立去抖**。`debounceMs` 是 per-handler 的，不是全局的。timers Map 按 `h.id` keyed，所以同一 handler 的连续事件被合并，不同 handler 各自独立。
2. **dispatcher 与 store 同生命周期**。`agent-session-store` 创建时建一个 dispatcher，销毁时 `dispatcher.destroy()` 清所有 timers（`plugins/registry.ts:66-69`）。这保证关 workspace 不会留定时器泄漏。

调度器从哪里被调用？`agent-session-store.ts` 处理 `item.completed` 事件时，匹配 `item.kind === 'tool_result'`，把输出挂到消息块的 tool 块上，然后调 `toolDispatcher.dispatch(ctx)`（约 `agent-session-store.ts:578`）。具体见 [02 章](./02-stores-and-session.md) 和 [03 章](./03-shell-and-chat.md)。

## 端到端时序：外部插件从清单到生效

```
1. main.tsx:31  installHost()
   └─ window.tos 就绪（注册三件套可用）

2. main.tsx:32  registerBuiltinPlugins()
   └─ 4 个内置插件 + 13 个工具渲染器进入注册表

3. main.tsx:33  loadExternalPlugins()
   ├─ fetchManifest() -> GET /api/plugins
   │   ├─ 401 (cookie 过期) -> 返回 null
   │   └─ manifestLoaded 保持 false，留待 AuthProvider 重试
   │
   └─ （假设 200 + 清单）
      ├─ for entry in 清单:
      │   ├─ eager: injectScript(bundleUrl) -> script 执行
      │   │   └─ IIFE 内部调 window.tos.registerPlugin/registerPanel/registerToolRenderer
      │   │       └─ 各 registry 立即更新
      │   │
      │   └─ lazy: registerLazyDescriptors(entry)
      │       ├─ registerLazyPanel(id, ...) -> 占位进 lazy-registry
      │       ├─ registerLazyToolRenderer(name, pluginId) -> 占位
      │       └─ registerLazyToolHandler(pattern, pluginId) -> 占位
      └─ manifestLoaded = true

4. AuthProvider mount -> GET /me 成功
   └─ 再次调 loadExternalPlugins()（如果第一次 401）
       └─ manifestLoaded 仍 false 时真去拉清单

5. 用户在 UI 里点了某个 lazy 面板
   └─ useWsApps 发现是 lazy -> ensurePluginLoaded(pluginId)
       └─ injectScript(bundleUrl) -> IIFE 执行
           └─ window.tos.registerPlugin({...})
               ├─ registerToolResultHandler 进 plugins/registry handlers[]
               └─ registerPanel 进 lib/panel-registry Map
       └─ 面板 component 真正渲染（替换占位）

6. SSE 流来 tool_result 事件
   └─ agent-session-store onItemCompleted
       └─ toolDispatcher.dispatch(ctx)
           ├─ matchLazyToolHandlers: 触发 lazy 插件加载（如有）
           └─ for h in handlers:
               └─ if h.match(ctx) -> h.onMatch(ctx) （或去抖后调）
```

## 延伸阅读建议

读完本章后：

- 想看一个真实的内置插件长什么样 -> 读 `plugins/files/index.ts`（最简单的，3 个 handler 都是 invalidate queries）
- 想看 lazy 注册表的细节 -> 读 `lib/plugin-lazy-registry.ts`
- 想理解 dispatcher 在 SSE 流里的位置 -> 跳到 [02 章](./02-stores-and-session.md) 看 agent-session-store 处理 `item.completed`

下一章 [02 状态层与会话双层架构](./02-stores-and-session.md) 讲消息和会话状态如何组织 - 这是整个项目的核心数据通路。
