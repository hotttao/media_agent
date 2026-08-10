# 04 端到端阅读路径

> 总章。把前三章的内容用三条端到端主线串起来。每条主线都从一个用户动作开始，列出涉及的文件、状态变化、跨层调用。这是复习用的章节 - 如果你能不查文档复述这三条路径，说明你掌握了这个项目的核心代码。

## 主线 1：浏览器加载到第一个画面

**场景**：用户在浏览器打开 `https://app.example.com/w/ws-1?session=sess-2`，从白屏到看到 chat 界面。

### 阶段 A：boot（React 渲染之前）

```
1. main.tsx:28  boot() 启动
   ├─ installChunkReload()                  // lib/chunk-reload.ts
   ├─ installHost()                         // lib/host.ts:81
   │   └─ window.tos = { React, registerPlugin, registerPanel, ... }
   ├─ registerBuiltinPlugins()              // plugins/index.ts:39
   │   ├─ registerPlugin(filesPlugin)
   │   ├─ registerPlugin(memoryPlugin)
   │   ├─ registerPlugin(skillsPlugin)
   │   ├─ registerPlugin(builderModePlugin)
   │   ├─ for 13 propose tools: registerToolRenderer(name, agentRequestProposeRenderer)
   │   └─ setSubAgentSessionLink(SubAgentSessionLink)
   └─ loadExternalPlugins()                  // lib/plugin-loader.ts:139
       ├─ fetchManifest() -> GET /api/plugins
       │   └─ 可能 401（cookie 过期），返回 null 不缓存
       └─ eager: 注入 <script> / lazy: 注册占位描述符
```

**关键**：`window.tos` 必须在外部插件 script 执行前就绪，否则插件调用 `window.tos.registerXxx` 会爆 `Cannot read property of undefined`。

### 阶段 B：React Provider 嵌套

```
2. main.tsx:34  createRoot().render(
   StrictMode >
     QueryClientProvider(queryClient) >
       ThemeProvider(defaultTheme="dark") >
         BrowserRouter >
           AuthProvider >                     // contexts/AuthContext.tsx
             TooltipProvider >
               App /                          // App.tsx
               Toaster
```

`AuthProvider` mount 时调 `api.getMe()`：

- 401 -> 跳 `/login?next=...`
- 200 -> set user，触发 `loadExternalPlugins()` 重试（如果阶段 A 是 401）

### 阶段 C：路由匹配

```
3. App.tsx:70-78  Route "/w/:workspaceId" 匹配
   └─ <ProtectedRoute> 检查 user
       └─ <Desktop>  // components/shell/Desktop.tsx
           │   scope = 'ws'（pathname 以 /w/ 开头）
           │   workspaceId = 'ws-1'
           │   useWsApps('ws-1') 拉 app 列表
           │   useActiveLayout('ws-1') 读布局
           │   useWorkspaceProfileLoaded('ws-1') -> false（首次）
           │
           └─ slotConfig = null（profile 没加载）
               └─ 不挂 SlotProvider，先渲染 shell 骨架
                   <DialogStackProvider>
                     <Menubar />
                     <main>{children}</main>  // children = <WorkspacePage />
                     <Dock />
                   </DialogStackProvider>

4. WorkspacePage.tsx mount
   └─ 拉 workspace 信息、session 列表（react-query）
       └─ useAutoSelectSession 选 sess-2（来自 URL ?session=）
       └─ 挂 <AgentSessionProvider workspaceId="ws-1" initialSessionId="sess-2">
           └─ stores/AgentSessionContext.tsx:68 创建 store
               ├─ createAgentSessionStore('ws-1', deps, { chatEndpoint })
               ├─ setState({ activeSessionId: 'sess-2' })
               └─ useActiveSession.switchTo('ws-1', 'sess-2')  // 写全局桥

5. WorkspaceProfileStore.ensureLoaded('ws-1')
   └─ GET /workspaces/ws-1/profile
       └─ set byWorkspace['ws-1'] = { slots: {...}, layout, ... }
           └─ profileLoaded = true
               └─ Desktop slotConfig 重新计算 -> 非空
                   └─ SlotProvider 挂上，slot 容器开始渲染

6. AgentSessionProvider Effect 触发
   └─ store.switchSession('sess-2', undefined)
       ├─ api.getWorkspaceMessages('ws-1', 'sess-2')  // 拉历史
       └─ 如果 chat_status === 'agent':
           └─ reconnect() -> POST /_proxy/agent/ws-1/cp-reconnect
               └─ SSE 流附加到进行中的 turn
```

### 阶段 D：第一个画面

```
7. SlotContainer 渲染 slot-a 的 active instance
   └─ 如果是 chat app:
       ├─ <AppWindow> 包 chrome
       │   └─ header 用 useAppHeaderSlot() portal 进 App 的标题/选择器
       └─ <App key={instance.id} instanceId={instance.id} />
           └─ Chat 组件读 useAgentSessionStore(s => s.messages)
               └─ SDK MessageBubble 渲染每条消息的 blocks
                   └─ TranscriptProviders 包 4 个 SDK provider
                       ├─ TranscriptI18nProvider
                       ├─ LazyToolRenderersProvider (桥到 plugin-lazy-registry)
                       ├─ AgentTypeProvider
                       └─ MarkdownProvider (用 app 的 Markdown)
                   └─ 每个 tool_call block 查工具渲染器（静态/lazy/外部）
```

**对应章节**：[00 总体架构](./00-overview.md)（启动时序与 Provider 嵌套）、[01 主机层](./01-host-and-plugins.md)（boot 阶段 A）、[02 状态层](./02-stores-and-session.md)（阶段 C/D 的 store 装配）、[03 Shell](./03-shell-and-chat.md)（阶段 C 的 slotConfig 与阶段 D 的渲染）。

---

## 主线 2：用户发消息到看到回包

**场景**：用户在 chat 输入框打 "hello"，按回车，看到流式回复。

```
1. Composer 组件
   └─ useAgentSessionActions().sendMessage('hello', undefined)
       └─ store.sendMessage 内部（agent-session-store.ts）

2. store.sendMessage:
   ├─ set({ isBusy: true, pendingMessage: null })
   ├─ messages.push({ role: 'user', blocks: [{ type: 'text', text: 'hello' }] })
   └─ deps.sse.createAgentChat('ws-1', { message: 'hello', session_id, source, images }, deps)

3. lib/api/sse.ts: createAgentChat
   ├─ fetch('/_proxy/agent/ws-1/chat', { method: POST, body, credentials: include })
   └─ consumeStream(response, handlers, signal, getSessionId, reconnect)
       └─ runTurn(response, { plugins: [handlersToPlugin(handlers, getSessionId)] })

4. SSE 事件流（每个事件触发 handlersToPlugin.onEvent）：

   4a. session.started
       └─ onSessionStarted(sessionId)
           └─ set activeSessionId（如果是新会话）
           └─ effects.onSessionCreated(sessionId)  // 让 teamwork 记录
           └─ Effect B 写 URL + 写全局 active-session-store

   4b. item.started (assistant 消息开始)
       └─ onItemStarted(item)
           └─ messages.push({ role: 'assistant', blocks: [], itemId })

   4c. item.delta (text 增量)
       └─ onItemDelta(itemId, { type: 'text', text: 'He' })
           └─ append 到对应 block
           └─ useAgentSessionStore(s => s.messages) 触发重渲染
               └─ SDK MessageBubble 流式更新
                   └─ TranscriptProviders 让 SDK 用对的 i18n/markdown/agentType/lazy renderers

   4d. item.completed (item 定稿)
       └─ onItemCompleted(item)
           └─ if item.kind === 'tool_result':
               ├─ attachToolResult(message, item)
               └─ toolDispatcher.dispatch(ctx)
                   ├─ matchLazyToolHandlers -> 可能触发 lazy 插件加载
                   └─ for h in handlers: if h.match -> h.onMatch(ctx)  // 可能去抖
                       └─ e.g. filesPlugin handler: invalidateQueries(['files'])
                           └─ react-query 重拉文件列表

   4e. question.requested (SSE 抛问题，*_propose 工具)
       └─ onQuestionRequested({ requestId, questions })
           └─ set({ pendingQuestion: ... })
           └─ UI 渲染 agentRequestProposeRenderer 卡片
               └─ 用户点 Approve:
                   └─ useAgentSessionActions().respondToQuestion(requestId, answers)
                       └─ api.respondToQuestion -> 后端继续 turn

   4f. session.ended
       └─ onSessionEnded(sessionId, reason, stats)
           ├─ set({ isBusy: false, lastTurnStats: stats })
           ├─ effects.onTurnComplete() -> playNotifySound()
           └─ effects.invalidateSessions('ws-1')  // react-query 失效

5. 用户看到完整回复，流式更新结束
```

**关键边界**：

- **网络挂时**：`sendMessage` 会先存 `pendingMessage`，后端可恢复时用 `setPendingMessage` API 同步
- **关页时**：`isBusy=true` 触发 `beforeunload` 拦截（`AgentSessionContext.tsx:148-157`）
- **切走会话时**：`switchSession` 会 abortStream，但后端 turn 继续；切回时 `reconnect()` 重连
- **chat_status === 'agent'**：会话有进行中 turn，需要 cp-reconnect（主线 1 阶段 C 已处理）

**对应章节**：[02 状态层](./02-stores-and-session.md)（store actions、SSE 事件处理）、[03 Shell](./03-shell-and-chat.md)（sse.ts、TranscriptProviders、工具渲染器三种注册协作）、[01 主机层](./01-host-and-plugins.md)（toolDispatcher 派发到插件 handler）。

---

## 主线 3：用户开一个新 App（面板）

**场景**：用户在 dock 点 "Files" app，看到它出现在 slot。

```
1. Dock.tsx 渲染 app 图标列表
   └─ useWsApps('ws-1') 返回 AppDefinition[]
       └─ 包含静态 app + lazy 插件声明的 panel 占位

2. 用户点击 Files 图标
   └─ ctx.ensureInstance('files')  // contexts/SlotContext.tsx:264
       ├─ for slot in layoutSlots:
       │   └─ if 已有 'files' 实例: return { slotId, instanceId }  // 跳转已有
       ├─ preferred = layoutSlots.find(s => s.defaultOpened?.includes('files'))
       ├─ fallback = layoutSlots.find(s => s.id === fallbackSlotId) ?? layoutSlots[0]
       ├─ slotId = preferred?.id ?? fallback
       └─ open(slotId, 'files')  // SlotContext.tsx:247
           ├─ cur = getState(slotId).opened
           ├─ fresh = { id: newInstanceId(), appId: 'files' }
           ├─ writeSlot(slotId, { opened: [...cur, fresh], active: fresh.id })
           │   └─ workspace-profile-store.patch('ws-1', { slots: ... })
           │       └─ 400ms 去抖 PATCH /workspaces/ws-1/profile
           └─ return fresh.id

3. workspace-profile-store 状态变化
   └─ useWorkspaceProfile('ws-1') 返回新 payload
       └─ SlotProvider 重渲染 -> SlotContainer 重渲染

4. SlotContainer.tsx 渲染 slot-a 的 active instance
   └─ state.active = { id: newInstanceId, appId: 'files' }
   └─ app = ctx.getApp('files') -> AppDefinition
   └─ <AppWindow onClose onPopOut onFill isFilled>
       ├─ header 渲染
       │   └─ <div ref={setHeaderSlot} /> 作为 portal target
       └─ <AppHeaderSlotContext.Provider value={headerSlot}>
           └─ <FilesApp key={instanceId} instanceId={instanceId} />  // app.Component
               └─ FilesApp 内部用 useAppHeaderSlot() 拿 div，createPortal 进去
                   └─ 渲染路径输入框、面包屑、操作按钮

5. FilesApp 读取自己的状态
   └─ useInstanceState(workspaceId, instanceId, 'cwd')  // 易失
   └─ useInstancePersistentState(workspaceId, instanceId, 'viewingPath')  // 持久
       └─ 后端 workspace_profile 实例状态字段

6. 如果 Files 是 lazy 插件声明的 panel（不是内置 app）：
   └─ useWsApps 检测到 lazyPanel 注册的占位
       └─ 用户点 dock 图标前先 ensurePluginLoaded(pluginId)
           └─ lib/plugin-loader.ts:105 injectScript(bundleUrl)
               └─ IIFE 执行 -> window.tos.registerPanel({ id: 'files', ... })
                   └─ panel-registry Map 更新，version++
                       └─ useWsApps 重渲染（subscribePanels 触发）
                           └─ 真正的 panel component 替换占位
```

**关键设计**：

- **`ensureInstance` 跳过 popout**：注释（`SlotContext.tsx:69-72`）说"popouts are explicit, not implicit"。markdown 链接、命令面板等外部调用 `ensureInstance` 时不会落到 popout。
- **`key={instance.id}` remount**：切换 active 实例时旧组件 unmount、新组件 mount。想跨切保留状态必须提到上层 store。
- **hidden app 不能 popIn**：注释（`SlotContext.tsx:336-339`）说 hidden app（如文件查看器）没有 dock/picker 入口，popIn 回去会变孤儿，所以只能在 popout 里活和死。
- **deterministic seed id**：`getState` 用 `${slotId}:${appId}` 合成种子 instance id，避免未写过时多次读生成不同 id 触发 remount（`SlotContext.tsx:199-202`）。

**对应章节**：[03 Shell](./03-shell-and-chat.md)（SlotContext 14 API、popout 虚拟 slot、AppWindow portal header、SlotContainer remount 语义）、[01 主机层](./01-host-and-plugins.md)（lazy 加载触发路径之一）、[02 状态层](./02-stores-and-session.md)（instance-state-store 与 workspace-profile-store 的协作）。

---

## 三条主线的交汇点

把三条主线放一起看，能发现项目里几个反复出现的设计原则：

### 原则 1：单源真相 + 多视图订阅

- slot 状态只在 `workspace_profile.slots`，不在 React state
- 会话状态只在 `agent-session-store`，不在组件 state
- 全局"当前会话"只在 `active-session-store`，不复制
- 工具渲染器只在 SDK 内部注册表，本仓库只调注册函数

**好处**：任何修改自动反映到所有订阅者，不需要手动同步。
**代价**：必须严格用 store API，不能在组件里 `useState` 缓存一份。

### 原则 2：用 vanilla store + Context 隔离重状态

- `agent-session-store` 是 per-workspace 隔离
- 双向订阅 + 早退检查防循环
- `useAgentSessionActions()` 返回稳定引用，做 useEffect 依赖不重渲染

**好处**：重状态不触发 Provider re-render，组件按 selector 订阅切片。
**代价**：心智模型比纯 `useState` 复杂，要理解双向同步。

### 原则 3：注册表 + useSyncExternalStore 桥接非 React 世界

- `panel-registry` 不是 React，但 `subscribePanels` + `getPanelsVersion` 让 React 在新面板注册时重渲染
- `plugin-lazy-registry` 同样模式
- `LazyToolRenderers` 桥把 lazy 注册表接入 SDK

**好处**：注册表是纯 TS 模块，可以在任何地方调（包括外部 script），不必走 React。
**代价**：每次注册要 bump version + 通知所有订阅者。

### 原则 4：刻意最小化主机表面

- `window.tos` 只放 nap 专属行为，不放展示层库
- 外部插件自己解决 shadcn/Radix/zustand/i18next
- 主机靠"必须共享 React"这一硬约束让插件能用 hooks

**好处**：主机不会膨胀，新插件不需要主机改代码就能用现有 API。
**代价**：插件作者要自己 import 展示层库（重复 bundle 大小）。

### 原则 5：失败不阻塞 boot

- 外部插件 401 不缓存空列表，留待 auth 后重试
- 单个 lazy 插件加载失败只 console.error，不抛
- 一个 lazy 插件首次匹配事件被丢，等加载完后续事件命中
- `manifestLoaded` + `inflight` 双重守卫让重入安全

**好处**：插件系统挂了不影响主功能。
**代价**：用户可能短暂看不到插件 app，需要交互触发加载。

---

## 自检清单

读完这套文档后，用以下问题自检：

- [ ] 说清 `window.tos` 的 5 个命名空间各放什么
- [ ] 画出 `main.tsx` 的 boot 4 步 + React Provider 嵌套树
- [ ] 解释为什么 `agent-session-store` 用 vanilla + Context 而不是 `create()`
- [ ] 说出 `active-session-store` 的 4 个字段
- [ ] 画双向同步的两个 useEffect 的数据流向，说明防循环检查在哪
- [ ] 列出 SlotContext 的 14 个 API，按"读/打开关闭/填充/popout"分组
- [ ] 解释 popout 为什么是虚拟 slot 而不是独立状态机
- [ ] 说清 `writeSlots` 解决的中间态问题
- [ ] 解释 `SlotContainer` 的 `key={instance.id}` 为什么导致 remount，跨切状态怎么保
- [ ] 说清 transcript 渲染委托给 SDK 后，app 通过 `TranscriptProviders` 注入了什么
- [ ] 列出 SSE 的 7 种事件类型 + 每种触发什么 store 操作
- [ ] 说清工具渲染器的三种注册如何协作（静态/lazy/外部）
- [ ] 说出 lazy 插件加载的三种触发路径
- [ ] 解释 `handlersToPlugin` 的三个防御性 flag（`sessionEndedDispatched` / `errorDispatched` / `aborted`）

如果以上都能答上来，你已经掌握了这个 web 项目的核心代码。剩下的就是按需查 API 表和 file:line 引用，去做实际的修改和扩展。

---

## 下一步学习建议

掌握核心后，按需深入：

1. **想扩展业务功能** -> 读 `hooks/` 对应领域的 hook（如 `useSkills.ts`），理解 react-query queryKey 命名约定
2. **想加新内置插件** -> 模仿 `plugins/files/index.ts` 写一个，注册到 `plugins/index.ts:42`
3. **想加新内置 App** -> 读 `lib/app-registry.ts` 和 `components/shell/apps/` 现有实现
4. **想做外部插件** -> 用 `window.tos.registerPanel` / `registerToolRenderer` / `registerPlugin`，参考 lazy manifest 的 `owns` 字段
5. **想改聊天交互** -> 读 `components/chat/tool-renderers/mcp/agent-request.tsx`，理解 Approve/Reject 卡如何触达 `useAgentSessionActions().respondToQuestion`
6. **想做 workspace 终端** -> 读 `components/workspace/WorkspaceTerminalPanel.tsx`（项目唯一的 WebSocket 使用点）
7. **想理解后端契约** -> 读 `lib/api/types.ts` + `lib/api/client.ts` 的方法签名，对照后端 control-plane 路由

每条深入路径都对应一个分章里的"延伸阅读建议"小节，按需查阅。

---

**回到**：[INDEX](./INDEX.md) | [00 总体架构](./00-overview.md) | [01 主机层](./01-host-and-plugins.md) | [02 状态层](./02-stores-and-session.md) | [03 Shell](./03-shell-and-chat.md) | [05 App Registry](./05-slots-and-app-registry.md)
