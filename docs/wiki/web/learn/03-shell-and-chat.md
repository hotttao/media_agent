# 03 Shell/Slot 模型与聊天渲染管线

> 分章。讲 Desktop 如何决定布局、SlotContext 的 14 个 API、popout 虚拟 slot 的设计、AppWindow 的 portal header、聊天 transcript 如何委托给 SDK 渲染、SSE 流如何消费。读完你应该能解释用户在 dock 点一个 app 到它出现在屏幕上发生了什么，以及用户发一条消息到看到回包的完整管线。

## 代码地图

### Shell 骨架

| 文件 | 大小 | 职责 |
|------|------|------|
| `components/shell/Desktop.tsx` | 6.4K | 根 Shell：scope 检测 + slotConfig 装配 + 渲染 Menubar/main/Dock/PopoutLayer/CommandPalette |
| `components/shell/Menubar.tsx` | 3.6K | 顶部栏 |
| `components/shell/Dock.tsx` | 25K | 底部停靠栏 |
| `components/shell/LayoutSwitcher.tsx` | 18K | 布局选择器 UI |
| `components/shell/PopoutLayer.tsx` | 11K | 浮动弹出窗口层 |
| `components/shell/SlotContainer.tsx` | 1.7K | 渲染单个 slot 的 active instance |
| `components/shell/SlotEmptyState.tsx` | 1.3K | 空 slot 占位 |
| `components/shell/SlotPicker.tsx` | 18K | 选 app 打开到 slot 的 UI |
| `components/shell/WsSwitcher.tsx` | 14K | workspace 切换器 |
| `components/shell/apps/` | 目录 | 内置 App 实现 |
| `components/shell/layout/` | 目录 | 布局定义（1col/2col/3col） |
| `components/shell/master-sidebar/` | 目录 | 主侧边栏 |
| `components/shell/windows/` | 目录 | AppWindow / AppHeaderButton |

### 上下文

| 文件 | 提供什么 |
|------|---------|
| `contexts/SlotContext.tsx` | SlotProvider + 14 个 API + POPOUT_SLOT_ID |
| `contexts/DialogStackContext.tsx` | 命名对话框栈 |
| `contexts/AuthContext.tsx` | 鉴权 |

### 聊天

| 文件 | 职责 |
|------|------|
| `components/chat/TranscriptProviders.tsx` | 把 4 个 SDK provider 接到 app 的状态 |
| `components/chat/SubAgentSessionLink.tsx` | call_agent 结果里的 session id 跳转链接 |
| `components/chat/VoiceInputButton.tsx` | 语音输入按钮 |
| `components/chat/tool-renderers/mcp/agent-request.tsx` | `*_propose` 工具的 Approve/Reject 卡 |
| `components/chat/tool-renderers/mcp/agent-request-bodies.tsx` | 卡片里按 kind 渲染 body |

### SSE / API

| 文件 | 职责 |
|------|------|
| `lib/api/client.ts` | `ApiClient` 主类（fetch 单例 `api`） |
| `lib/api/sse.ts` | SSE 流消费（用 `@neutree-ai/sse-consumer` 的 `runTurn`） |
| `lib/api/types.ts` | 类型 |
| `lib/api/channel-gateway.ts` | `/_cg/` 端点 |
| `lib/api/agent-files.ts` | Agent 文件系统 API |

## 抓手拆解

### 抓手 1：Desktop 的 scope 检测

`components/shell/Desktop.tsx:60`：

```typescript
const scope: Scope = location.pathname.startsWith('/w/') ? 'ws' : 'fleet'
```

一句话：URL 以 `/w/` 开头就是 workspace scope，否则是 fleet。这个 scope 决定了：

- 用哪个 profileId（`workspaceId` vs `FLEET_PROFILE_ID`，line 66）
- 用哪个 app 源（`useWsApps(workspaceId)` vs `useFleetApps()`，line 62-63）
- 用哪个 slot 配置（布局文件 vs 硬编码 fleet slot，line 75-91）
- PopoutLayer 挂哪（fleet 在 Desktop 里挂，ws 在 WorkspacePage 里挂，line 158-162）

### 抓手 2：slotConfig 的延迟装配

`Desktop.tsx:75-91`：

```typescript
const slotConfig = useMemo(() => {
  if (!profileLoaded) return null          // 等 profile 拉到再装配
  const fallbackSlotId = getFallbackSlotId(layoutId)
  if (scope === 'ws' && workspaceId) {
    return { slots: LAYOUTS[layoutId].slots, apps: wsApps, id: workspaceId, fallbackSlotId }
  }
  if (scope === 'fleet') {
    const slots = layoutId === '1col' ? FLEET_SLOTS_1COL : FLEET_SLOTS_2COL
    return { slots, apps: fleetApps, id: FLEET_PROFILE_ID, fallbackSlotId }
  }
  return null
}, [scope, workspaceId, profileLoaded, wsApps, fleetApps, layoutId])
```

**关键设计**：profileLoaded 之前 slotConfig 是 `null`，`Desktop.tsx:148-174` 的渲染分支：

```jsx
<DialogStackProvider>
  {slotConfig ? (
    <SlotProvider ...>
      {shell}
      {scope === 'fleet' && <PopoutLayer />}
      {palette}
    </SlotProvider>
  ) : (
    <>
      {shell}
      {palette}
    </>
  )}
</DialogStackProvider>
```

profile 没加载时不挂 SlotProvider，避免用默认 slot 闪一下再切到用户保存的 slot。注释（line 73-74）原话：

> Suppress slot/layout/dock rendering until the profile has been fetched at least once - otherwise we'd flash defaults before snapping to the user's saved choice.

### 抓手 3：SlotConfig 的字段

`contexts/SlotContext.tsx:10-20`：

```typescript
interface SlotConfig {
  /** Stable id used to key into the slot map. e.g., 'slot-a'. */
  id: string
  /**
   * App ids opened on first visit when the workspace profile has no entry
   * for this slot. Instance ids are generated lazily on first read. The
   * first item is treated as the active default; the rest are pre-loaded
   * so the slot's tab strip surfaces them right away.
   */
  defaultOpened?: string[]
}
```

**注意**：`defaultOpened` 只在"用户从未配置过这个 slot"时作为种子。一旦用户改了 slot，存到 `workspace_profile.slots`，`defaultOpened` 就被忽略。

Fleet 的两个 slot 配置（`Desktop.tsx:33-46`）：

```typescript
const FLEET_SLOTS_2COL: SlotConfig[] = [
  { id: 'slot-a', defaultOpened: ['workspaces', 'library', 'connectors', 'credentials', 'models'] },
  { id: 'slot-b', defaultOpened: ['activity'] },
]
const FLEET_SLOTS_1COL: SlotConfig[] = [
  { id: 'slot-a', defaultOpened: ['workspaces', 'library', 'connectors', 'credentials', 'models', 'activity'] },
]
```

注释说 2col 是历史默认：slot-a 是 launcher 列，slot-b 是 Activity 边车。1col 折叠成单列，Activity 仍在 dock 可聚焦。

### 抓手 4：SlotContext 的 14 个 API

`contexts/SlotContext.tsx:48-119` 是接口定义。按职责分组：

| API | 签名 | 用途 |
|-----|------|------|
| **状态读** | | |
| `workspaceId` | `string` | Provider 绑定的 profileId |
| `slots` | `SlotConfig[]` | 布局槽列表（不含 popout） |
| `apps` | `AppDefinition[]` | 可用 app 列表 |
| `getApp` | `(appId) => AppDefinition \| undefined` | 查 app 定义 |
| `getState` | `(slotId) => SlotState` | 读 slot 的 `{opened, activeId, active}` |
| **打开/关闭** | | |
| `open` | `(slotId, appId) => instanceId` | 总是新建实例并激活 |
| `ensureInstance` | `(appId) => {slotId, instanceId}` | 已存在则跳转，否则开新（跳过 popout） |
| `activate` | `(slotId, instanceId)` | 切到已打开的实例 |
| `close` | `(slotId, instanceId)` | 关实例（删除其状态），活跃则前移 |
| **布局填充** | | |
| `filledSlot` | `string \| null` | 非 null 时只渲染这一个 slot |
| `setFilledSlot` | `(slotId \| null)` | 切换填充模式 |
| **Popout** | | |
| `popOut` | `(slotId, instanceId)` | 把 slot 里的实例移到浮动层 |
| `popIn` | `(instanceId)` | 从浮动层回到布局槽 |
| `openInPopout` | `(appId, seedPersistent?) => instanceId` | 直接在浮动层开新实例 |
| `moveInstance` | `(srcSlotId, destSlotId, instanceId, destIndex)` | 跨槽移动或同槽重排 |
| `popoutGeo` | `PopoutGeo \| null` | 浮动窗位置 |
| `setPopoutGeo` | `(geo)` | 改浮动窗位置 |

### 抓手 5：popout 虚拟 slot 的设计

`contexts/SlotContext.tsx:38-46`：

```typescript
/**
 * The popout layer is modeled as a virtual slot named `popout`. It uses the
 * same `{ opened, active }` shape as layout slots, so all SlotContext
 * mechanics (open/activate/close, instance state cleanup) reuse identically.
 * It just isn't rendered by any layout - PopoutLayer reads it directly and
 * draws a floating window with tab consolidation across all popped-out
 * instances.
 */
export const POPOUT_SLOT_ID = 'popout'
```

**这是关键抽象**：popout 不是另立一套状态机，而是复用 slot 模型。`SlotProvider` 内部把 `popout` 加到 slots 列表（`SlotContext.tsx:183` 的 `[...layoutSlots, { id: POPOUT_SLOT_ID }]`），但对外只暴露 layout slots（`SlotContext.tsx:418` 的 `slots: layoutSlots`）。

好处：

- `popOut` / `popIn` / `moveInstance` 都是 slot 之间的数据搬运，逻辑完全一致
- instance id 跨 popout/slot 保留不变，所以 `instance-state-store` 里的状态不丢，组件不 remount
- `PopoutLayer` 自己读 `getState(POPOUT_SLOT_ID)` 渲染浮动窗，把多个 popped-out 实例合并成 tab

### 抓手 6：状态持久化到 workspace_profile

`SlotProvider` 不维护 React state，所有 `{opened, active}` 都直接读写 `workspace-profile-store`（`SlotContext.tsx:215-245` 的 `writeSlot` / `writeSlots`）：

```typescript
const writeSlot = useCallback((slotId, patch) => {
  const profile = useWorkspaceProfileStore.getState()
  const cur = profile.byWorkspace[workspaceId]?.slots
  const nextEntry = { ...(cur?.[slotId] ?? {}), ...patch }
  const nextSlots = { ...(cur ?? {}), [slotId]: nextEntry }
  profile.patch(workspaceId, { slots: nextSlots })  // 400ms 去抖 PATCH 后端
}, [workspaceId])
```

**单源真相**：注释（line 161-170）强调：

> Single source of truth - no URL or ls shadowing. Survives refresh / ls clear / cross-device. Per-instance UI state (scroll, cwd, etc.) lives in the separate instance state store, keyed by `AppInstance.id`.

`getState`（`SlotContext.tsx:190-213`）的逻辑：

1. 从 `storedSlots` 读存储的 `opened` 与 `active`
2. 如果没有，从 `cfg.defaultOpened` 用**确定性**的 instance id 合成种子：`${slotId}:${appId}`（line 199-202）。确定性是为了"未写过时多次读返回同一 id"，否则每次读生成新 id 会让 React `key` 变化触发 remount。
3. 过滤掉不存在的 app（`getApp(i.appId)` 找不到）
4. `activeId` 优先取存储值，否则取 `opened[0]`

### 抓手 7：跨槽位原子写

`SlotContext.tsx:233-245` 的 `writeSlots`：

```typescript
const writeSlots = useCallback((patches: Record<string, StoredSlot>) => {
  const profile = useWorkspaceProfileStore.getState()
  const cur = profile.byWorkspace[workspaceId]?.slots ?? {}
  const next = { ...cur }
  for (const [slotId, patch] of Object.entries(patches)) {
    next[slotId] = { ...(cur[slotId] ?? {}), ...patch }
  }
  profile.patch(workspaceId, { slots: next })
}, [workspaceId])
```

**为什么需要它**：`popOut` 把实例从 slot-a 移到 popout，要写两个 slot。如果用两次 `writeSlot`：

1. 写 slot-a（删除实例）-> React 重渲染 -> `SlotContainer` 看到这个实例不在了 -> unmount
2. 写 popout（加实例）-> React 重渲染 -> `SlotContainer` 看到 popout 有新实例 -> mount 新组件

中间这一帧"实例不存在"会导致组件 unmount 再 mount，状态丢失。`writeSlots` 把两个 patch 合成一次 PATCH + 一次本地 state 更新，避免中间态。

注释（line 226-232）原话：

> Multi-slot atomic write. Used by popOut/popIn so removing from one slot and adding to another is one debounced PATCH (and one local-state update), avoiding a transient state where the instance exists in neither - which the `key={instance.id}` remount in SlotContainer would otherwise interpret as "instance gone, unmount".

### 抓手 8：SlotContainer 的 remount 语义

`components/shell/SlotContainer.tsx:17-51`：

```typescript
export function SlotContainer({ slotId }: SlotContainerProps) {
  const ctx = useRequiredSlotContext()
  const state = ctx.getState(slotId)
  const instance = state.active
  if (!instance) return <SlotEmptyState slotId={slotId} />
  const app = ctx.getApp(instance.appId)
  if (!app) return <SlotEmptyState slotId={slotId} />
  const App = app.Component
  const isFilled = ctx.filledSlot === slotId
  function handleFill() { ctx.setFilledSlot(isFilled ? null : slotId) }
  return (
    <AppWindow onClose={...} onPopOut={...} onFill={handleFill} isFilled={isFilled}>
      <App key={instance.id} instanceId={instance.id} />
    </AppWindow>
  )
}
```

**关键注释**（line 9-16）：

> Each opened-but-inactive app is NOT mounted here - switching active is a remount. Apps that need to preserve cross-switch state (e.g., chat sessions) rely on context providers rendered higher up the tree.

也就是说：

- slot 里只有 `active` 的实例被挂载，其它 inactive 的实例不挂载
- 切换 active 就是 unmount 旧 + mount 新（用 `key={instance.id}` 强制）
- 想保留跨切的状态，得把状态提到上层（如 `AgentSessionProvider`、`instance-state-store`）

这个设计简化了渲染（不需要虚拟 DOM 切换），代价是 inactive 实例的本地 React state 丢失。所以 chat 的会话状态必须在 store 里（不能在组件 state 里），切换会话不丢消息。

### 抓手 9：AppWindow 的 portal header

`components/shell/windows/AppWindow.tsx:100-124`：

```jsx
<header className="flex h-9 ...">
  {/* App header portal target */}
  <div ref={setHeaderSlot} className="flex min-w-0 flex-1 items-center gap-1.5" />
  {/* Window controls */}
  <WindowControls onClose={...} onPopOut={...} onFill={...} ... />
</header>
<AppHeaderSlotContext.Provider value={headerSlot}>
  <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
</AppHeaderSlotContext.Provider>
```

**设计**：

- header 左侧的 div 用 `ref={setHeaderSlot}` 拿到 DOM 节点
- 这个节点通过 `AppHeaderSlotContext.Provider` 暴露给 children
- App 组件用 `useAppHeaderSlot()` 拿到节点，再 `createPortal(content, node)` 把自己的标题/选择器/操作渲染进 header

这样 App 自包含（自己定义 header 内容），但 Shell 控制窗口 chrome（边框、阴影、窗口控件）。注释（line 38-46）说：

> Convention: render business actions left-aligned (the slot is a left-flex region). Do not pad with counts / status strings to "fill" the header - empty space is fine. Window controls live to the right and are managed by the shell.

窗口控件（`AppHeaderButton`）四个：popOut / fill / fullscreen / close，hover 时显示（`opacity-0 group-hover/appwindow:opacity-100`，line 152-155）。`window.tos.windows.useAppHeaderSlot` 和 `window.tos.windows.AppHeaderButton` 就是给外部插件用的（`lib/host.ts:63-64`），让它们也能在 header 注入内容。

### 抓手 10：聊天 transcript 委托给 SDK

`components/chat/TranscriptProviders.tsx:24-48`：

```typescript
const lazyToolRenderers: LazyToolRenderers = {
  getPluginId: getLazyToolRenderer,
  load: ensurePluginLoaded,
  subscribe: subscribeLazyRegistry,
  getVersion: getLazyRegistryVersion,
}

export function TranscriptProviders({ agentType, children }: { agentType: string; children: ReactNode }) {
  const { i18n } = useTranslation()
  return (
    <TranscriptI18nProvider locale={i18n.language}>
      <LazyToolRenderersProvider value={lazyToolRenderers}>
        <AgentTypeProvider value={agentType}>
          <MarkdownProvider value={Markdown}>{children}</MarkdownProvider>
        </AgentTypeProvider>
      </LazyToolRenderersProvider>
    </TranscriptI18nProvider>
  )
}
```

**关键事实**：transcript 的实际渲染**不在本仓库**，在 `@neutree-ai/ui-sdk` 的 `MessageBubble` 组件里。`SharePage` 直接 import 它（`pages/SharePage.tsx:9`），workspace 的 chat 也用它（在 `components/shell/apps/` 的 chat app 里）。

这个 provider 组件只做 4 件事，把 app 状态注入 SDK：

| Provider | 注入什么 |
|----------|---------|
| `TranscriptI18nProvider locale={i18n.language}` | 让 SDK 用 app 的当前语言 |
| `LazyToolRenderersProvider value={lazyToolRenderers}` | **关键**：把 lazy 工具渲染器桥接给 SDK |
| `AgentTypeProvider value={agentType}` | 让 SDK 知道当前 agent 类型（影响渲染逻辑） |
| `MarkdownProvider value={Markdown}` | 让 SDK 用 app 自己的 Markdown 组件（带文件链接集成） |

**LazyToolRenderers 桥的 4 个 callback**（line 24-29）：

- `getPluginId(toolName)` - 查这个工具名是不是 lazy 插件声明的，是则返回 pluginId
- `load(pluginId)` - 触发 `ensurePluginLoaded`（注入 plugin bundle）
- `subscribe(cb)` - 注册表变化时通知 SDK（`subscribeLazyRegistry`）
- `getVersion()` - 单调计数器，SDK 用 `useSyncExternalStore` 检测变化

SDK 拿到这些 callback 后，渲染 tool_call block 时：先查 `getPluginId`，没有就用静态注册的渲染器；有就调 `load` 触发插件加载，加载完插件调 `window.tos.registerToolRenderer` 真注册，version++ 让 SDK 重渲染。

### 抓手 11：SSE 流消费

`lib/api/sse.ts:1-2`：

```typescript
import { type TurnPlugin, runTurn } from '@neutree-ai/sse-consumer'
```

**不是原生 `EventSource`**。SDK 的 `runTurn` 用 `fetch()` + `ReadableStream` 解析 SSE 协议。原因：

- 原生 EventSource 不支持 POST + body（只能 GET + query）
- 原生 EventSource 不支持自定义 header（cookie 可以但要走 fetch 才能统一）
- `runTurn` 提供了 `TurnPlugin` 钩子，可以拦截每个事件做适配

`AgentSSEHandler`（`sse.ts:38-48`）是 app 侧的回调包：

| 回调 | 触发时机 |
|------|---------|
| `onSessionStarted(sessionId)` | `session.started` 事件 |
| `onSessionEnded(sessionId, reason, stats)` | `session.ended` 事件 |
| `onItemStarted(item)` | `item.started` 事件 |
| `onItemDelta(itemId, delta)` | `item.delta` 事件 |
| `onItemCompleted(item)` | `item.completed` 事件 |
| `onQuestionRequested(request)` | `question.requested` 事件 |
| `onError(error)` | error 事件 + 流错误 |

`handlersToPlugin`（`sse.ts:60-122`）把 `AgentSSEHandler` 适配成 `TurnPlugin`，加防御：

- `sessionEndedDispatched` - `session.ended` 已发后，`onError` 不再触发（避免重复报错）
- `errorDispatched` - 服务器发了 `error` 事件后，流的 `onError` 不再触发
- `err.reason === 'aborted'` - 用户主动 abort 不算错
- **session_id 不匹配的事件被丢弃**（line 74-75）：cp-reconnect 是 session 作用域的（query param 带 `session_id`），但流里可能有跨 session 的 stray 事件，防御性过滤

### 抓手 12：`createAgentChat` 与 reconnect

`lib/api/sse.ts` 的两个入口：

- `createAgentChat(workspaceId, payload, deps)`（约 line 224）- 发新 turn：
  - `POST /_proxy/agent/:workspaceId/chat`（或自定义 chatEndpoint）
  - body: `{ message, session_id, source, images }`
  - 响应用 `consumeStream` 消费

- `createCPReconnectStream(workspaceId, sessionId, deps)`（约 line 286）- 重连进行中的 turn：
  - `POST /_proxy/agent/:workspaceId/cp-reconnect`
  - 用于页面刷新后重连到 `chat_status === 'agent'` 的会话
  - 退避策略（`buildCPReconnectFactory` line 179-220）：500ms 起、8s 上限、5 次重试

### 抓手 13：工具渲染器的三种注册

回顾 [01 章](./01-host-and-plugins.md)，工具渲染器有三种注册方式，它们在 transcript 渲染时协作：

| 注册方式 | 谁注册 | 存在哪 | 何时生效 |
|----------|--------|--------|---------|
| 静态 `registerToolRenderer(name, Component)` | 内置插件（`plugins/index.ts:46-48`）| `@neutree-ai/ui-sdk` 内部 | 立即 |
| Lazy `registerLazyToolRenderer(toolName, pluginId)` | 外部 lazy 插件清单（`plugin-loader.ts:188`）| `lib/plugin-lazy-registry.ts` | 用户首次渲染该工具时触发 `ensurePluginLoaded` |
| 外部 script 直接调 `window.tos.registerToolRenderer` | 外部 eager 插件 IIFE | SDK 内部 | script 执行完立即 |

渲染时 SDK 先查静态注册表（立即用），找不到查 `getLazyToolRenderer`，有就触发 `load`，加载完插件真注册，version++ 触发重渲染。这条协作链的关键是把 `LazyToolRenderers` 桥传给 SDK（`TranscriptProviders.tsx:24-29` + `:41`）。

## 端到端时序：用户发消息到看到回包

下面把所有抓手串起来。场景：用户在 `/w/ws-1` 的 chat app 里输入 "hello" 按回车。

```
1. Composer 组件读 useAgentSessionActions().sendMessage
   └─ sendMessage('hello', undefined)
       ├─ set({ isBusy: true, pendingMessage: null })
       ├─ messages.push({ role: 'user', blocks: [{ type: 'text', text: 'hello' }] })
       └─ deps.sse.createAgentChat('ws-1', { message: 'hello', session_id, source, images }, deps)

2. lib/api/sse.ts: createAgentChat
   ├─ fetch('/_proxy/agent/ws-1/chat', { method: POST, body, credentials: include })
   └─ consumeStream(response, handlers, signal, getSessionId, reconnect)
       └─ runTurn(response, { plugins: [handlersToPlugin(handlers, getSessionId)] })

3. @neutree-ai/sse-consumer: runTurn 解析 SSE 流，按事件触发 plugin.onEvent
   └─ handlersToPlugin.onEvent(evt):
       └─ switch(evt.type):

4. evt.type === 'session.started'  (新会话创建)
   └─ handlers.onSessionStarted(sessionId)
       └─ store 收到：set activeSessionId、set loadedSessionId
       └─ effects.onSessionCreated(sessionId)  // 让 teamwork 等记录新 session
       └─ Effect B 触发：写 URL ?session=xxx + 写 active-session-store

5. evt.type === 'item.started'  (assistant 消息开始)
   └─ handlers.onItemStarted(item)
       └─ messages.push({ role: 'assistant', blocks: [], itemId: item.item_id })

6. evt.type === 'item.delta'  (text 增量)
   └─ handlers.onItemDelta(itemId, { type: 'text', text: 'He' })
       └─ 找到对应 message，append 到 text block
       └─ useAgentSessionStore(s => s.messages) 触发重渲染 -> SDK MessageBubble 重渲染
       └─ TranscriptProviders 包的 4 个 provider 让 SDK 用对的 i18n/markdown/agentType/lazy renderers

7. evt.type === 'item.completed'  (item 定稿)
   └─ handlers.onItemCompleted(item)
       └─ 如果 item.kind === 'tool_result':
           ├─ attachToolResult(message, item)  // 把 output 挂到 tool block
           └─ toolDispatcher.dispatch({
           │     toolName, toolInput, toolOutput, isError, callId, workspaceId
           │   })
           ├─ matchLazyToolHandlers(toolName) -> 可能触发 lazy 插件加载
           └─ for h in handlers:
               └─ if h.match(ctx): h.onMatch(ctx)  // 或去抖后调
                   └─ e.g. filesPlugin handler: invalidateQueries(['files'])
                       └─ react-query 重新拉文件列表

8. evt.type === 'question.requested'  (SSE 抛出问题)
   └─ handlers.onQuestionRequested({ requestId, questions })
       └─ set({ pendingQuestion: { ... } })
       └─ UI 渲染 *_propose 工具的 Approve/Reject 卡（agentRequestProposeRenderer）
           └─ 用户点 Approve -> useAgentSessionActions().respondToQuestion(requestId, answers)
               └─ api.respondToQuestion('ws-1', sessionId, requestId, answers)
                   └─ 后端继续 turn，下一波 SSE 流来

9. evt.type === 'session.ended'  (turn 结束)
   └─ handlers.onSessionEnded(sessionId, reason, stats)
       ├─ set({ isBusy: false, lastTurnStats: stats })
       ├─ effects.onTurnComplete() -> playNotifySound()  // 叮
       └─ effects.invalidateSessions('ws-1')  // react-query 失效会话列表

10. 用户看到完整回复，UI 流式更新结束
    └─ 如果用户关页：beforeunload 拦截（isBusy=true）
    └─ 如果用户切走会话：abortStream() 掐 SSE，但后端继续跑，回来 reconnect()
```

## 延伸阅读建议

读完本章后：

- 想看 PopoutLayer 如何合并多实例成 tab -> 读 `components/shell/PopoutLayer.tsx`
- 想看布局定义 -> 读 `components/shell/layout/layouts.ts` 与同目录布局组件
- 想看一个完整的 chat app 实现 -> 读 `components/shell/apps/` 下的 chat app（具体文件名待你探索）
- 想看 MCP 工具渲染卡 -> 读 `components/chat/tool-renderers/mcp/agent-request.tsx` + `agent-request-bodies.tsx`
- 想看 channel-gateway API -> 读 `lib/api/channel-gateway.ts`
- 想看 workspace 终端的 WebSocket -> 读 `components/workspace/WorkspaceTerminalPanel.tsx`（唯一的 WebSocket 使用点）

下一章 [04 端到端阅读路径](./04-reading-paths.md) 用三条主线把所有章节串起来 - 这是复习用的"总"章。
