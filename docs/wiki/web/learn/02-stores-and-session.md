# 02 状态层与会话双层架构

> 分章。讲 stores/ 的整体设计、为什么用 vanilla zustand + Context 双层、active-session-store 如何做桥、agent-session-store 的状态形状与 actions、双向同步的两个 useEffect、其他全局 store 的角色。读完你应该能解释一条用户消息从输入到流式回包的状态流转。

## 代码地图

`stores/` 目录（约 129KB 总）：

| 文件 | 大小 | 库 | 作用域 |
|------|------|----|--------|
| `stores/active-session-store.ts` | 2.6K | zustand `create` | 全局单例 |
| `stores/agent-session-store.ts` | 35K | zustand/vanilla `createStore` | 每 workspace 一个 |
| `stores/AgentSessionContext.tsx` | 7.4K | React Context + `useStore` | Provider 包 vanilla store |
| `stores/workspace-profile-store.ts` | 4.3K | zustand `create` | 全局单例（按 workspaceId namespace） |
| `stores/instance-state-store.ts` | 11K | zustand `create` + `useSyncExternalStore` | 全局单例（按 workspaceId+instanceId namespace） |
| `stores/composer-store.ts` | 4.1K | zustand `create` | 全局单例 |
| `stores/draft-store.ts` | 2.6K | zustand `create` + `persist` | 全局单例（localStorage `tos-chat-drafts`） |
| `stores/markdown-preferences-store.ts` | 442B | zustand `create` + `persist` | 全局单例（localStorage `tos-markdown-prefs`） |
| `stores/fleet-profile.ts` | 2.4K | 常量 + ProfileBackend | fleet scope 的伪 workspaceId |

`contexts/` 三个：

- `AuthContext.tsx` - 鉴权
- `DialogStackContext.tsx` - 命名对话框栈
- `SlotContext.tsx` - slot 状态（见 [03 章](./03-shell-and-chat.md)）

## 抓手拆解

### 抓手 1：两种 zustand 用法

项目里 zustand 有两种用法，理解区别是理解整个状态层的前提：

**用法 A：`create()` 全局单例**

```typescript
// 典型：active-session-store.ts:39
export const useActiveSession = create<ActiveSessionState>((set) => ({
  workspaceId: undefined,
  sessionId: undefined,
  switchTo: (wid, sid) => set({ workspaceId: wid, sessionId: sid }),
}))
```

- 整个 app 一个 store 实例。
- 直接 `useActiveSession(selector)` 即可在任何组件读。
- 适合：跨 workspace 的轻量状态、全局 UI 偏好。

**用法 B：`createStore()`（vanilla）+ React Context**

```typescript
// 典型：agent-session-store.ts 用 createStore 创建
const store = createStore<AgentSessionSlice>((set, get) => ({ ... }))

// AgentSessionContext.tsx:68-107 在 useState 里创建
const [store] = useState(() => {
  const s = createAgentSessionStore(workspaceId, deps, { chatEndpoint })
  return s
})
```

- 每个 Provider 一个独立 store 实例。
- 通过 Context 暴露，组件用 `useStore(store, selector)` 读。
- 适合：重状态、按 workspace 隔离、可能同时存在多份。

**为什么 agent-session 用 vanilla + Context 而不是 `create()`？**

- 一个 user 可以同时打开多个 workspace tab（虽然实际只有一个 AgentSessionProvider 挂载），状态必须按 workspace 隔离。
- `create()` 是全局单例，做不到 per-workspace 隔离。
- 但又不能用 `useState` 直接放 state（state 太重，每次 set 触发整个 Provider 重渲染不可接受）。
- vanilla store + Context 是 React 官方推荐的重状态模式：state 在 store 里（不触发 Provider re-render），组件按 selector 订阅切片。

### 抓手 2：会话双层架构

这是本章核心。会话状态被拆成两层：

```
┌──────────────────────────────────────────────────────┐
│ 全局层：active-session-store (zustand create)        │
│   { workspaceId, sessionId, context, switchTo }      │
│   - 轻：只记"当前哪个会话"                            │
│   - 任何组件可读（侧边栏、命令面板、URL 同步）       │
└────────┬─────────────────────────────────────────────┘
         │ 双向订阅（两个 useEffect）
┌────────▼─────────────────────────────────────────────┐
│ Provider 层：agent-session-store (zustand vanilla)   │
│   - 重：messages、SSE 流、loading、busy、pending...  │
│   - 每 workspace 一个，挂在 AgentSessionProvider 下  │
│   - 组件用 useAgentSessionStore(selector) 订阅切片   │
└──────────────────────────────────────────────────────┘
```

**为什么需要这两层？**

- `AgentSessionProvider` 只在 `/w/:workspaceId` 路由下挂载。但侧边栏、命令面板、URL 同步这些组件在 Provider 之外（比如 fleet scope 也有侧边栏），它们也需要知道"当前哪个会话"。
- 全局 `active-session-store` 就是这个桥：它在 Provider 之外可读可写，Provider 内部订阅它来同步自己的 `activeSessionId`。
- 同时，Provider 内部状态变化（用户在 chat 里切了 session）也要写回全局，让侧边栏高亮跟着变。

### 抓手 3：`active-session-store` 的极简 API

`stores/active-session-store.ts` 全部状态：

```typescript
interface ActiveSessionState {
  workspaceId: string | undefined
  sessionId: string | undefined
  context?: { sessionChatStatus?: string; lastTurnStats?: any }
  switchTo(workspaceId: string, sessionId: string | undefined, context?: any): void
}
```

**就这几个字段**。这是设计上的克制 - 全局层只放"哪个会话"，不放会话内容。

`useSessionNavigation()` hook（`active-session-store.ts:57-67`）：

```typescript
export function useSessionNavigation() {
  return { switchToSession(workspaceId: string, sessionId: string) { ... } }
}
```

只暴露一个动作，专门给"切到某会话"场景。`window.tos.workspace.useSessionNavigation` 就是它（`lib/host.ts:96`），外部插件用它来"跳到某会话"。

### 抓手 4：`AgentSessionProvider` 装配

`stores/AgentSessionContext.tsx:50-168` 的 Provider 干 6 件事：

1. **创建 store**（line 68-107）：用 `useState` 一次性建一个 `createAgentSessionStore(workspaceId, deps, opts)`。`deps` 注入三类依赖：
   - `api` - 7 个 REST 方法（getWorkspaceMessages、getSession、setPendingMessage、respondToQuestion、interruptSession、deleteSession 等）
   - `sse` - `createAgentChat` + `createCPReconnectStream`
   - `effects` - `onSessionCreated`、`onTurnComplete`（播提示音）、`invalidateSessions`、`invalidateWorkspaces`（react-query cache 失效）
2. **初始化 `activeSessionId`**（line 102-105）：同步 setState，让首渲染就有正确值。
3. **加载历史**（line 111-117）：mount effect 里调 `store.switchSession(initialSessionId, initialContext)` 拉历史。
4. **双向同步**（line 119-145）：两个 useEffect，下面单独讲。
5. **`beforeunload` 拦截**（line 148-157）：`isBusy` 时阻止关页（避免 SSE 流被掐断丢消息）。
6. **卸载 abort**（line 160-165）：unmount 时 `store.getState().abortStream()` 取消进行中的 SSE。

### 抓手 5：双向同步的两个 useEffect

这是双层架构的精髓。两个订阅互相对称：

**Effect A：全局 -> Provider**（`AgentSessionContext.tsx:120-129`）

```typescript
useEffect(() =>
  useActiveSession.subscribe((curr, prev) => {
    if (curr.workspaceId !== workspaceId) return          // 别的 workspace 的切换，忽略
    if (curr.sessionId === prev.sessionId && curr.workspaceId === prev.workspaceId) return
    if (curr.sessionId === store.getState().activeSessionId) return  // 自己写的回声，忽略
    store.getState().switchSession(curr.sessionId, curr.context)  // 真正切
  }),
[store, workspaceId])
```

侧边栏点击会话 -> `useActiveSession.switchTo(wid, sid)` -> 这个订阅触发 -> store 切会话 -> 加载历史、重置 SSE。

**Effect B：Provider -> 全局 + URL**（`AgentSessionContext.tsx:136-145`）

```typescript
useEffect(() =>
  store.subscribe((state, prev) => {
    if (state.activeSessionId !== prev.activeSessionId) {
      syncRef.current(state.activeSessionId)             // 同步到 URL（?session=xxx）
      useActiveSession.getState().switchTo(workspaceId, state.activeSessionId)  // 同步到全局
    }
  }),
[store, workspaceId])
```

store 内部 `activeSessionId` 变（用户在 chat 里新建会话、删除会话、SSE 报告新 session）-> 这个订阅触发 -> 写 URL + 写全局。

**防循环依赖**：两个订阅都有"如果值没变就 return"的早退检查（Effect A 的 `curr.sessionId === store.getState().activeSessionId`、Effect B 的 `state.activeSessionId !== prev.activeSessionId`）。否则 A 触发 B 触发 A 死循环。

### 抓手 6：`AgentSessionSlice` 状态形状

`stores/agent-session-store.ts:108-132` 的状态字段：

| 字段 | 类型 | 含义 |
|------|------|------|
| `workspaceId` | `string` | 当前 workspace |
| `activeSessionId` | `string \| undefined` | 当前会话 id |
| `loadedSessionId` | `string \| undefined` | 已加载历史的会话 id（避免重复 load） |
| `messages` | `ChatMessage[]` | 消息列表 |
| `isLoading` | `boolean` | 加载历史中 |
| `isSwitching` | `boolean` | 切会话中 |
| `isDeleting` | `boolean` | 删会话中 |
| `isBusy` | `boolean` | SSE 流进行中（用来拦 beforeunload） |
| `error` | `string \| null` | 错误 |
| `pendingQuestion` | `Question \| null` | SSE 抛出的问题（用户需 Approve/Reject） |
| `pendingMessage` | `string \| null` | 待发消息（网络挂时缓存） |
| `lastTurnStats` | `object \| undefined` | 上一轮 token 用量等 |

`ChatMessage` 形状（在 `lib/api/types.ts`）：每条消息有 `blocks: ContentBlock[]`，block 类型有 `text` / `tool_call` / `tool_result` / `status` / `image`。

### 抓手 7：actions 清单

`stores/agent-session-store.ts:134-169` 暴露的 actions：

| Action | 用途 |
|--------|------|
| `switchSession(sessionId, context)` | 切会话：abort 旧 SSE、reset state、load 历史 |
| `sendMessage(text, images?)` | 发新消息：开新 turn 的 SSE 流 |
| `sendMessageToSession(sessionId, text, images?)` | 发到指定 session（不切） |
| `updatePendingMessage(text)` / `clearPendingMessage()` | 编辑/清待发消息 |
| `respondToQuestion(responseId, answers)` | 回答 SSE 抛出的问题 |
| `stop()` | 用户点停止 -> 后端 interrupt |
| `abortStream()` | 仅 abort SSE（不做后端 cleanup） |
| `deleteSession(sessionId)` | 删会话 |
| `reconnect()` | 重连进行中的 turn（页面刷新后用） |
| `loadHistory(sessionId)` | 拉历史消息 |
| `clearMessages()` | 清空当前 messages |
| `destroy()` | 清订阅 + abort SSE + destroy dispatcher |

**`useAgentSessionActions()`**（`AgentSessionContext.tsx:182-185`）的精妙之处：

```typescript
export function useAgentSessionActions() {
  // Actions are stable references on the zustand store - read once, never changes.
  return useStoreApi().getState()
}
```

zustand vanilla store 的 `getState()` 返回的对象引用在 store 创建后稳定不变（actions 是闭包绑死的）。所以 `useAgentSessionActions()` 返回的整个 actions 对象**永远不会变**，组件用它做 useEffect 依赖不会重渲染。这是把 vanilla store 包成 Context 的额外好处。

### 抓手 8：SSE 流处理（简版，详见 03 章）

`stores/agent-session-store.ts` 处理 SSE 事件的入口是 `consumeStream()`，把 `lib/api/sse.ts` 的回调适配成 store 操作。事件类型：

| SSE 事件 | store 操作 |
|----------|-----------|
| `session.started` | 新会话创建：set activeSessionId、调 `effects.onSessionCreated` |
| `session.ended` | turn 结束：set `isBusy=false`、调 `effects.onTurnComplete`（播提示音）、`invalidateSessions` |
| `item.started` | 新 item 开始：push 一个空 block 到当前 message |
| `item.delta` | item 增量：append 到对应 block |
| `item.completed` | item 完成：定稿 block；如果是 `tool_result`，调 `toolDispatcher.dispatch(ctx)`（见 [01 章](./01-host-and-plugins.md)） |
| `question.requested` | 设置 `pendingQuestion`（前端弹 Approve/Reject 卡） |
| `error` | set `error`、set `isBusy=false` |

**`item.completed` 与工具结果调度器的衔接**（约 `agent-session-store.ts:570-590`）：

```typescript
if (item.kind === 'tool_result') {
  // 把 output 挂到消息的 tool block 上
  attachToolResult(message, item)
  // 触发插件 handler（带去抖）
  toolDispatcher.dispatch({
    toolName, toolInput, toolOutput, isError, callId, workspaceId
  })
}
```

这就是 [01 章](./01-host-and-plugins.md) 的 dispatcher 真正被调用的地方。SSE 流 -> store -> dispatcher -> 各 plugin handler。

### 抓手 9：其他全局 store 的角色

| Store | 何时用 | 持久化 |
|-------|--------|--------|
| `workspace-profile-store` | 每 workspace 的布局/slot 配置/wallpaper/popoutGeo。`ensureLoaded(wid)` 拉 `GET /workspaces/:id/profile`，`patch(wid, patch)` 400ms 去抖写。支持 `registerProfileBackend` 替换后端（fleet-profile 用这个机制）。 | 后端 |
| `instance-state-store` | 每 AppInstance 的 UI 状态（滚动位置、cwd 等）。两个 hook：`useInstanceState`（内存）和 `useInstancePersistentState`（DB 后端，通过 workspace profile）。 | 内存 / 后端 |
| `composer-store` | "往输入框插文本"的跨面板通道。`requestInsert(workspaceId, text)` 一次，chat 组件订阅消费。 | 内存 |
| `draft-store` | 草稿。`setDraft(key, value)` / `clearDraft(key)`。key 是 `workspaceId:sessionId`。 | localStorage |
| `markdown-preferences-store` | 仅一个字段 `tocVisible`。 | localStorage |
| `fleet-profile.ts` | 不是 store，是常量 `FLEET_PROFILE_ID = '__fleet__'` + 注册一个 ProfileBackend 让 fleet scope 复用 workspace-profile-store。 | n/a |

### 抓手 10：`useHasAgentSessionProvider` 的回退设计

`AgentSessionContext.tsx:193-195`：

```typescript
export function useHasAgentSessionProvider(): boolean {
  return useContext(AgentSessionCtx) !== null
}
```

为什么需要这个？因为 `SharePage`（公开分享页）不在 `AgentSessionProvider` 下，但用 SDK 的 `MessageBubble` 渲染 transcript，里面可能有 `*_propose` 工具调用。这些工具的渲染器（`agentRequestProposeRenderer`）会调 `useAgentSessionActions()` 来触发 approve/reject - 在 SharePage 里没有 provider 就会抛错。

`useHasAgentSessionProvider` 让渲染器检测"我在不在 provider 里"，不在就回退到只读渲染（不显示按钮）。这是**渐进降级**模式，让同一套渲染器既能在工作区交互也能在公开页只读。

## 端到端时序：用户切换会话

下面这条路径把双层架构的所有部分串起来。场景：用户在 `/w/ws-1` 聊天，点击侧边栏的另一个会话 `sess-2`。

```
1. 侧边栏 SessionItem onClick
   └─ activeSessionStore.switchTo('ws-1', 'sess-2')
       └─ useActiveSession.set({ workspaceId: 'ws-1', sessionId: 'sess-2' })

2. AgentSessionProvider Effect A 触发（订阅了 useActiveSession）
   ├─ 检查 workspaceId === 'ws-1' ✓
   ├─ 检查 sessionId !== prev.sessionId ✓
   ├─ 检查 'sess-2' !== store.getState().activeSessionId ✓
   └─ store.switchSession('sess-2', undefined)

3. store.switchSession 内部（agent-session-store.ts）
   ├─ if (activeSessionId) abortStream()    // 掐当前 SSE
   ├─ set({ isSwitching: true, messages: [], activeSessionId: 'sess-2' })
   ├─ store.subscribe 触发 Effect B:
   │   ├─ state.activeSessionId 变了
   │   ├─ syncRef.current('sess-2')         // 写 URL ?session=sess-2
   │   └─ activeSessionStore.switchTo('ws-1', 'sess-2')  // 写回全局
   │       └─ Effect A 收到这个事件，但检查发现 sessionId === store.activeSessionId，早退
   │       （防循环）
   ├─ loadHistory('sess-2')
   │   ├─ api.getWorkspaceMessages('ws-1', 'sess-2')
   │   └─ set({ messages: [...], loadedSessionId: 'sess-2', isSwitching: false })
   └─ 如果 sess-2 的 chat_status === 'agent'：
       └─ reconnect() -> POST /_proxy/agent/ws-1/cp-reconnect
           └─ SSE 流重新附加到进行中的 turn
```

整个过程：**全局层一次 set -> Provider Effect A 触发 store 切换 -> store Effect B 写回全局与 URL**。双向同步在两个订阅里闭环。

## 延伸阅读建议

读完本章后：

- 想看 SSE 流的完整处理 -> 读 `lib/api/sse.ts` 的 `consumeStream` 与 `handlersToPlugin`
- 想看 reconnect 的退避策略 -> 读 `lib/api/sse.ts:179-220` 的 `buildCPReconnectFactory`
- 想看 messages 的具体形状 -> 读 `lib/api/types.ts` 的 `ChatMessage` / `ContentBlock`
- 想看 dispatcher 如何在 store 创建时绑定 -> 读 `agent-session-store.ts` 找 `createToolResultDispatcher()` 调用处

下一章 [03 Shell/Slot 模型与聊天渲染管线](./03-shell-and-chat.md) 讲 UI 怎么把这些状态渲染出来，以及 Slot 模型如何管理多窗口。
