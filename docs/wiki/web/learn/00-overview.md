# 00 总体架构

> 总章。先给一张代码地图让你建立坐标，再画启动时序与 Provider 嵌套树，最后落到"三层架构模型"这个抓手。读完这章你应该能在脑里画出 web/src 的目录骨架，并能解释每一层干什么。

## 代码地图

`web/src/` 一级目录就是分层。按职责分组：

### 启动与路由（顶层）

| 文件 | 职责 |
|------|------|
| `main.tsx` | 启动入口。装 4 个全局子系统 → 渲染 React Provider 树 |
| `App.tsx` | 路由表。`/login` `/oauth/authorize` `/` `/w/:workspaceId` `/s/:shareId` `/invite/:token` |
| `index.css` | 全局样式 + Tailwind |
| `vite-env.d.ts` | Vite 类型声明 |

### lib（基础设施层）

`lib/` 是**与业务无关的工具与单例**。最关键的几个：

| 文件 | 职责 |
|------|------|
| `lib/host.ts` | `window.tos` 全局主机对象，外部插件唯一的入口 |
| `lib/host-events.ts` | 轻量事件总线（`on`/`emit`） |
| `lib/panel-registry.ts` | 面板注册表（`Map<id, PluginPanel>` + 订阅） |
| `lib/plugin-lazy-registry.ts` | 延迟插件的占位描述符表 |
| `lib/plugin-loader.ts` | 外部插件加载器（eager/lazy 两种模式） |
| `lib/app-registry.ts` | 内置 App 定义（区别于插件 Panel，是 Desktop 的 dock 应用） |
| `lib/api/` | API 客户端：`client.ts` 主类 + `sse.ts` 流式 + `types.ts` + `channel-gateway.ts` + `agent-files.ts` |
| `lib/i18n.ts` | i18next 初始化 |
| `lib/chunk-reload.ts` | 热更新 chunk 失效时的自动刷新 |
| `lib/utils.ts` | 通用工具（`cn` className 合并等） |

注意：`lib/` **不放业务状态**。状态在 `stores/`，业务 UI 在 `components/`。

### stores（状态层）

`stores/` 全部基于 zustand，分两类：

| 文件 | 作用域 | 用途 |
|------|--------|------|
| `stores/active-session-store.ts` | 全局单例 | "当前激活哪个会话" - 跨 Provider 的桥 |
| `stores/agent-session-store.ts` | 每 workspace 一个 | 重状态：消息、SSE 流、loading |
| `stores/AgentSessionContext.tsx` | React Context | 把上面的 vanilla store 包成 Provider + 暴露 hooks |
| `stores/workspace-profile-store.ts` | 全局单例 | 每 workspace 的布局/slot/壁纸配置 |
| `stores/instance-state-store.ts` | 全局单例 | 每 AppInstance 的易失/持久化 UI 状态 |
| `stores/composer-store.ts` | 全局单例 | "往输入框插入文本"的跨面板通道 |
| `stores/draft-store.ts` | 全局单例 + persist | 草稿（localStorage `tos-chat-drafts`） |
| `stores/markdown-preferences-store.ts` | 全局单例 + persist | markdown 偏好（localStorage `tos-markdown-prefs`） |
| `stores/fleet-profile.ts` | 常量 + ProfileBackend | fleet scope 的伪 workspaceId（`__fleet__`） |

注意是 **vanilla zustand + Context** 混用，不是清一色 `create()`。原因见 [02 章](./02-stores-and-session.md)。

### contexts（React Context 层）

| 文件 | 提供什么 |
|------|----------|
| `contexts/AuthContext.tsx` | `{ user, isLoading, login, logout, refreshUser }` |
| `contexts/DialogStackContext.tsx` | 命名对话框栈 `open(key)` / `close(key)` / `isOpen(key)` |
| `contexts/SlotContext.tsx` | **最重**：slot 的开/关/激活/popout/move 全套 API + 持久化 |

### plugins（插件层）

| 文件 | 职责 |
|------|------|
| `plugins/index.ts` | `registerBuiltinPlugins()` - 注册 4 个内置插件 + 13 个工具渲染器 |
| `plugins/registry.ts` | 插件注册表 + `createToolResultDispatcher()` |
| `plugins/types.ts` | `WorkspacePlugin` / `ToolResultHandler` / `ToolMatchCtx` 类型 |
| `plugins/files/` | 文件变更刷新信号 |
| `plugins/skills/` | 技能目录变更刷新 |
| `plugins/builder-mode/` | plan/command/config/prompt 库刷新 |
| `plugins/memory/` | 占位（无 handler、无 panel） |

### components（UI 层）

按场景分子目录，重要的几个：

| 目录 | 用途 |
|------|------|
| `components/shell/` | Desktop/Menubar/Dock/LayoutSwitcher/PopoutLayer/SlotContainer + `apps/`（应用实现）+ `windows/`（窗口框架）+ `layout/`（布局定义） |
| `components/chat/` | 聊天相关：TranscriptProviders、SubAgentSessionLink、VoiceInputButton、`tool-renderers/mcp/` |
| `components/ui/` | shadcn/Radix 基础组件（button/dialog/input/...） |
| `components/dialogs/` | 命名对话框实现 + `registry.ts` 注册 |
| `components/home/` | 首页 token 使用卡片 |
| `components/library/` | 技能/提示词/模板库 |
| `components/management/` | 凭证/应用/服务令牌/标签/团队管理 |
| `components/integration/` | 连接器集成 |
| `components/preferences/` | 偏好设置（壁纸、通知） |
| `components/resource/` | 通用资源卡片/列表/网格 |
| `components/admin/` | 管理面板 |
| `components/workspace/` | workspace 内部组件（终端、文件查看器等） |
| `components/teamwork/` | 团队协作时间线 |

### hooks（业务 hooks）

`hooks/` 下按领域分文件，每个文件导出 1-N 个 `useXxx`。常见的命名前缀：`useWorkspaces`、`useSessions`、`useSkills`、`useProviders`、`useSchedules`、`useEnvironments`、`useCommands`、`usePrompts`、`useTemplates`、`useTags`、`useBrowsers`、`useSandboxes`。

这些 hook **大多基于 TanStack Query**，query key 命名约定见各文件。

### pages（路由级页面）

| 文件 | 路由 | 一句话 |
|------|------|--------|
| `pages/LoginPage.tsx` | `/login` | 用户名密码 + 企业微信 SSO 入口 |
| `pages/OAuthAuthorizePage.tsx` | `/oauth/authorize` | OAuth 同意页 |
| `pages/FleetPage.tsx` | `/` | 登录后首页，fleet scope 的 slot 布局 |
| `pages/WorkspacePage.tsx` | `/w/:workspaceId` | workspace 主壳，挂 `AgentSessionProvider` |
| `pages/SharePage.tsx` | `/s/:shareId` | 公开只读会话分享（无 session provider） |
| `pages/InvitePage.tsx` | `/invite/:token` | 团队邀请接受页 |

### locales

i18next 翻译资源。按语言分文件。

---

## 启动时序

入口在 `main.tsx:28-52` 的 `boot()`：

```
boot()
  ├── installChunkReload()        // 1. 热更新 chunk 失效保护
  ├── installHost()               // 2. 装 window.tos 全局主机
  ├── registerBuiltinPlugins()   // 3. 注册 4 个内置插件 + 13 个工具渲染器
  ├── loadExternalPlugins()       // 4. 拉 /api/plugins 清单，注入外部插件 script
  │       └── （可能 401，留待 AuthProvider 重试）
  └── createRoot(...).render(     // 5. 渲染 React Provider 树
        StrictMode
          QueryClientProvider      // TanStack Query
            ThemeProvider           // 主题
              BrowserRouter        // 路由
                AuthProvider        // 鉴权（mount 时拉 /me）
                  TooltipProvider
                    App             // 路由表
                    Toaster         // sonner 通知
```

关键设计点：

1. **`installHost` 在 React 渲染之前**。外部插件 script 注入后会立刻调用 `window.tos.registerXxx`，必须保证 `window.tos` 已就绪。
2. **`loadExternalPlugins` 在鉴权之前**。`/api/plugins` 可能 401，loader 会返回 `null`（不缓存空列表），AuthProvider 拿到用户后会再次触发它（见 `lib/plugin-loader.ts:136-176` 的 `manifestLoaded` / `inflight` 守卫）。
3. **`registerBuiltinPlugins` 用 `registered` 标志位防重入**（`plugins/index.ts:37-41`），即使被调多次也只注册一次。
4. **`StrictMode` 在 dev 下会双调 effect**，所以所有 store 订阅/取消订阅必须严格成对。看 `AgentSessionContext.tsx` 的 `useEffect` 都是返回清理函数。

## Provider 嵌套树

`App.tsx` 路由表里的 `<Desktop>` 包了 `<ProtectedRoute>`，最终 UI 树大致是：

```
<QueryClientProvider>
  <ThemeProvider>
    <BrowserRouter>
      <AuthProvider>
        <TooltipProvider>
          <Routes>
            <Route path="/" element={
              <ProtectedRoute>
                <Desktop>           // Shell 入口
                  <FleetPage />
                </Desktop>
              </ProtectedRoute>
            } />
            <Route path="/w/:workspaceId" element={
              <ProtectedRoute>
                <Desktop>
                  <WorkspacePage />  // 内部挂 AgentSessionProvider + SlotProvider
                </Desktop>
              </ProtectedRoute>
            } />
            ...
          </Routes>
          <Toaster />
        </TooltipProvider>
      </AuthProvider>
    </BrowserRouter>
  </ThemeProvider>
</QueryClientProvider>
```

`Desktop.tsx:148-174` 自身又包了：

```
<DialogStackProvider>
  <SlotProvider workspaceId slots apps fallbackSlotId>   // 仅 profileLoaded 后挂
    <Menubar />
    <main>
      <ChunkErrorBoundary>
        <Suspense fallback={loading}>
          {children}            // FleetPage / WorkspacePage
        </Suspense>
      </ChunkErrorBoundary>
    </main>
    <Dock />
    {scope === 'fleet' && <PopoutLayer />}
    <CommandPalette />
  </SlotProvider>
</DialogStackProvider>
```

**WorkspacePage 内部**（不在 Desktop 里）会再挂一层 `AgentSessionProvider` + 工作区版的 `PopoutLayer`，因为 popout 窗口里的聊天类 App 需要找到 session context（见 `Desktop.tsx:158-162` 的注释）。

## 三层架构模型（抓手）

把上面所有信息压缩成一个心智模型，整张图就三层 + 一条状态桥：

```
┌─────────────────────────────────────────────────────────────┐
│  UI 层（components/ + pages/）                              │
│  Desktop / SlotContainer / AppWindow / 各 page / 对话框     │
│        ↑ 读 store            ↑ 用 hook                     │
└──────┬──────────────────────────────────┬──────────────────┘
       │                                  │
┌──────▼─────────────────┐    ┌───────────▼──────────────────┐
│ 状态层（stores/ +       │    │ 业务 hooks（hooks/）         │
│  contexts/）            │    │  useWorkspaces / useSessions │
│  active-session-store   │◄──►  useSkills / useProviders ...│
│  agent-session-store    │    │  （大多基于 TanStack Query） │
│  SlotContext            │    └──────────────────────────────┘
│  workspace-profile 等  │
└────────┬───────────────┘
         │
┌────────▼───────────────────────────────────────────────────┐
│  主机层 + 注册表层（lib/host.ts + lib/*-registry.ts +      │
│    plugins/registry.ts + lib/plugin-loader.ts）            │
│                                                            │
│  window.tos ──┬─ registerPlugin   ──► plugins/registry.ts  │
│               ├─ registerPanel    ──► lib/panel-registry.ts│
│               └─ registerToolRenderer ──► @neutree-ai/    │
│                                          ui-sdk registry   │
│                                                            │
│  外部插件 ──（script tag 注入）──► 调用 window.tos.register*│
└────────────────────────────────────────────────────────────┘
         │
         ▼  fetch /api/*  +  SSE 流
   后端 control-plane / agent
```

### 三层的边界规则

1. **UI 层不直接调 API**。要数据用 `hooks/` 里的 `useXxx`（Query），要发命令也走 hooks 暴露的 mutation；要读/写会话状态走 `useAgentSessionActions` / `useAgentSessionStore`。
2. **状态层不渲染业务 UI**。stores 暴露的是状态与 action，不是组件。例外是 `AgentSessionContext.tsx` / `SlotContext.tsx` 这种 Provider 组件本身，但它们只渲染 children，不渲染业务内容。
3. **主机层不持有业务状态**。`window.tos` 是表面，真正存数据的是 stores。主机只是把 stores 的 hooks 转发给外部插件用。
4. **注册表层用 `useSyncExternalStore` 接入 React**。panel-registry 不是 React 模块，但通过 `subscribePanels` + `getPanelsVersion` 让 React 在新面板注册时重渲染（见 `lib/panel-registry.ts:45-53`）。

### 为什么是这个分层

- **主机层在前**：因为外部插件 script 在 React 渲染前就执行，主机必须先就绪。
- **状态层用 vanilla zustand + Context**：会话状态重（messages 列表、SSE 流），用 vanilla store 避免 React re-render 雪崩；用 Context 限定作用域到 workspace。
- **全局 active-session-store 桥**：侧边栏、URL 同步、命令面板等组件在 `AgentSessionProvider` 之外，需要全局轻量状态来通信。

---

## 阅读建议

读完本章后，按下面顺序进入分章：

1. 先读 [01 主机层与插件三件套](./01-host-and-plugins.md) - 理解 `window.tos` 注册 API 与加载机制，这是后续所有插件代码的入口
2. 再读 [02 状态层与会话双层架构](./02-stores-and-session.md) - 理解消息如何流转，这是项目的核心数据通路
3. 然后读 [03 Shell/Slot 模型与聊天渲染管线](./03-shell-and-chat.md) - 理解 UI 怎么把状态层渲染出来
4. 最后读 [04 端到端阅读路径](./04-reading-paths.md) - 用三条主线把所有章节串起来验证理解

如果只读一章，读 02。状态层是这个项目的灵魂。
