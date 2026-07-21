# UI 页面骨架与布局搭建指南

> 面向"第一次打开项目不知道屏幕上每一块是谁渲染的"的新人。给一张完整页面骨架图，标注每个区域对应哪个组件，再以资深工程师视角讲解：如果从零开始搭这个前端，应该按什么顺序、每一步解决什么问题。
>
> 读完后你应该能：指着屏幕上任意一块区域，说出它由哪个组件渲染、数据从哪来、受谁控制。

## 一、页面骨架全景图

以 workspace 3 列布局（`3col`，默认布局）为例，整个屏幕从上到下分三层：**Menubar（顶栏）→ 主画布（Slot 布局）→ Dock（底栏）**，另有两个浮层（PopoutLayer / CommandPalette）覆盖在最上面。

```
┌───────────────────────────────────────────────────────────────────────┐
│  Menubar (h-10, border-b)                                  Desktop.tsx:124
│  ┌─────┐  ┊  ┌──────────────┐                ┌──────────┐ ┌────────┐  │
│  │Logo │  ┊  │  WsSwitcher  │                │ ⌘K Search│ │ User   │  │
│  └─────┘  ┊  └──────────────┘                └──────────┘ └────────┘  │
│         Menubar.tsx:18                                  CommandPalette │
│                                                        Button + UserBtn│
├───────────────────────────────────────────────────────────────────────┤
│  main (flex-1, min-h-0)                                    Desktop.tsx:129
│  ┌──────────────┬──────────────────────────┬──────────────────┐       │
│  │  slot-a      │  slot-b                  │  slot-c          │       │
│  │  (sessions)  │  (files/skills/...)      │  (chat)          │       │
│  │  固定宽 左    │  flex-1 自适应           │  固定宽 右       │       │
│  │              │                          │                  │       │
│  │ ┌──────────┐ │ ┌──────────────────────┐ │ ┌──────────────┐ │       │
│  │ │AppWindow │ │ │      AppWindow       │ │ │  AppWindow   │       │
│  │ │┌───────┐ │ │ │┌──────────────────┐ │ │ │┌────────────┐│       │
│  │ ││AppHdr │ │ │ ││  App Header Slot │ │ │ ││ App Header ││       │
│  │ ││portal │ │ │ ││  (portal target) │ │ │ ││  portal    ││       │
│  │ │└───────┘ │ │ │└──────────────────┘ │ │ │└────────────┘│       │
│  │ │          │ │ │                      │ │ │              │       │
│  │ │ Sessions │ │ │  Files / Skills /   │ │ │   Chat       │       │
│  │ │  App     │ │ │  Terminal / ...     │ │ │   App        │       │
│  │ │ Component│ │ │  App Component      │ │ │  Component   │       │
│  │ │          │ │ │                      │ │ │              │       │
│  │ └──────────┘ │ └──────────────────────┘ │ └──────────────┘       │
│  └─────┬────────┘ └─────┬───────────┬──────┘ └────┬─────────┘       │
│        │         GripVertical        │      GripVertical              │
│        └── 拖拽手柄 (w-2) ────────────┘ └── 拖拽手柄 (w-2) ────┘       │
│   ThreeColumnLayout.tsx:62                                          │
├───────────────────────────────────────────────────────────────────────┤
│  Dock (footer, rounded-full)                              Dock.tsx:646
│  ┌─────────────────────────────────────────────────────────────┐     │
│  │ [Layout]│ [slot-a pills] │ [slot-b pills] │ [slot-c pills] │     │
│  │ Switcher│  Sessions●     │  Files Skills  │  Chat          │     │
│  └─────────────────────────────────────────────────────────────┘     │
│   LayoutSwitcher + DockSegment×N (SortablePill + SlotPicker)         │
└───────────────────────────────────────────────────────────────────────┘

         ╔═══════════════════════════════════════╗
         ║  浮层（绝对定位，z-index 高）          ║
         ║  1. PopoutLayer - 拖出来的浮窗       ║  PopoutLayer.tsx:27
         ║  2. CommandPalette - ⌘K 命令面板     ║  CommandPalette.tsx
         ║  3. DialogStack - 对话框堆叠          ║  DialogStackContext.tsx
         ╚═══════════════════════════════════════╝
```

## 二、组件分层地图

把上面图里每个区域对应的组件、数据来源、控制方整理成表。**这是查表用的：看到屏幕上一块东西，先来这张表找对应。**

### 2.1 顶栏 Menubar

| 区域 | 组件 | 文件:行 | 数据来源 | 说明 |
|------|------|---------|----------|------|
| 整个顶栏 | `Menubar` | `shell/Menubar.tsx:18` | - | `h-10` 高度，`backdrop-blur` 毛玻璃 |
| 左侧 Logo | `Logo` + `Link to="/"` | `Menubar.tsx:28-34` | 静态 | 点击回首页（fleet scope） |
| 工作区切换器 | `WsSwitcher` | `shell/WsSwitcher.tsx:36` | `useWorkspaces()` / `useTags()` / `useUnreadCount()` | 下拉选 workspace，显示运行状态点 |
| 中间搜索框 | `CommandPaletteButton` | `Menubar.tsx:46` | - | 点击触发 `onOpenCommandPalette`，显示 `⌘K` 提示 |
| 右侧用户 | `UserButton` | `Menubar.tsx:76` | `useAuth().user` | 点击打开 `PreferencesDialog` |

### 2.2 主画布 main

`main` 标签本身在 `Desktop.tsx:129`，内部根据路由渲染不同页面：

| 路由 | 渲染的页面组件 | 内部布局 | 文件 |
|------|----------------|----------|------|
| `/` | `FleetPage` | `ActiveLayout`（1col/2col） | `pages/FleetPage.tsx:7` |
| `/w/:workspaceId` | `WorkspacePage` | `AgentSessionProvider` 包 `ActiveLayout` | `pages/WorkspacePage.tsx:17` |

**布局组件**（`ActiveLayout`）根据 `layoutId` 切换：

| layoutId | 组件 | slot 数 | slot-a | slot-b | slot-c | 文件 |
|----------|------|---------|--------|--------|--------|------|
| `1col` | `OneColumnLayout` | 1 | chat（全屏） | - | - | `layout/OneColumnLayout.tsx` |
| `2col` | `TwoColumnLayoutDefault` | 2 | files/browser/skills... | chat | - | `layout/TwoColumnLayout.tsx` |
| `3col` | `ThreeColumnLayout` | 3 | sessions | files/browser/skills... | chat | `layout/ThreeColumnLayout.tsx` |

每个 layout 的本质是：**几个 `<SlotContainer>` 加上拖拽手柄（`GripVertical`）的 flex 排列**。slot 宽度由 `useResizablePanel` 管理，持久化到 localStorage。

### 2.3 SlotContainer → AppWindow → App

这是主画布里**最核心的三层嵌套**，理解它就理解了整个布局：

```
SlotContainer (slotId)            shell/SlotContainer.tsx:17
  └─ ctx.getState(slotId).active   // 取当前激活的 AppInstance
     └─ ctx.getApp(instance.appId) // 取 AppDefinition
        └─ AppWindow               shell/windows/AppWindow.tsx:60
           ├─ <header>
           │   ├─ <div ref={setHeaderSlot} />  ← AppHeaderSlotContext.Provider
           │   └─ <WindowControls />          ← 关闭/弹出/填充/全屏按钮
           └─ <AppHeaderSlotContext.Provider value={headerSlot}>
              └─ {children}  ← <App key={instance.id} instanceId={instance.id} />
```

| 层 | 组件 | 职责 | 关键设计 |
|----|------|------|----------|
| SlotContainer | `SlotContainer` | 读 slot 当前激活实例，渲染对应 App | `key={instance.id}` 切实例会 remount |
| AppWindow | `AppWindow` | 提供 chrome（边框/圆角/阴影/header/控件） | header 左侧是 portal target，App 往里塞内容 |
| App | `app.Component` | 具体业务（Chat/Files/Sessions/...） | 通过 `useAppHeaderSlot()` 拿 portal target |

**AppWindow header 的 portal 模式**：AppWindow 在 header 里放一个空 `<div ref={setHeaderSlot}>`，并通过 `AppHeaderSlotContext.Provider` 把这个 div 暴露给子组件。App 组件内部用 `useAppHeaderSlot()` 拿到这个 div，再 `createPortal(jsx, div)` 把自己的标题/选择器/操作按钮渲染进去。这样 chrome（外壳）由 shell 统一控制，内容由 App 自治。

### 2.4 底栏 Dock

| 区域 | 组件 | 文件:行 | 说明 |
|------|------|---------|------|
| 整个 footer | `DockShell` | `Dock.tsx:643` | `rounded-full` 胶囊形，毛玻璃背景 |
| 布局切换 | `LayoutSwitcher` | `shell/LayoutSwitcher.tsx` | 1col/2col/3col 切换按钮 |
| slot 分段 | `DockSegment` × N | `Dock.tsx:396` | 每个 slot 一段，显示已打开实例的 pill |
| 实例 pill | `SortablePill` | `Dock.tsx:468` | 可拖拽排序，点击激活实例 |
| 添加实例 | `SlotPicker` | `shell/SlotPicker.tsx` | `+` 号弹出 App 列表 |
| 溢出折叠 | `SegmentOverflow` | `Dock.tsx` | 宽度不够时收起多余的 pill |

**Dock 的容量计算**：`distributeCapacity` 函数（`Dock.tsx:68`）根据视口宽度预算，按"最宽的先收起"策略裁剪每个 slot 显示几个 pill。拖拽时展开全部 pill 作为 drop target。

### 2.5 浮层

| 浮层 | 组件 | 文件 | 触发方式 | 挂载位置 |
|------|------|------|----------|----------|
| 浮动窗口 | `PopoutLayer` | `shell/PopoutLayer.tsx:27` | AppWindow 的"弹出"按钮 | fleet: Desktop 直接挂；ws: WorkspacePage 挂在 AgentSessionProvider 内 |
| 命令面板 | `CommandPalette` | `CommandPalette.tsx` | `⌘K` / 顶栏搜索按钮 | Desktop 内（SlotProvider 里） |
| 对话框堆叠 | `DialogStack` | `DialogStackContext.tsx` | 各种业务弹窗 | `DialogStackProvider` 包在最外层 |

PopoutLayer 是**绝对定位的浮窗**，有自己的拖拽/resize/tab strip/header portal，不套 AppWindow（避免双层 chrome）。它读 `POPOUT_SLOT_ID` 这个虚拟 slot 的 `opened` 列表，渲染所有弹出的实例。

## 三、布局是怎么一层层搭起来的

以资深工程师视角，讲这个布局从零到完整的搭建顺序。**每一步只解决一个问题，每步的产物可独立验证。**

### Step 0：确定设计语言与约束

在写任何 JSX 之前，先回答这几个问题：

1. **这是什么形态的应用？** → 类 IDE 的多面板桌面布局，不是传统网页滚动流
2. **视口怎么用？** → `h-svh`（100svh）满屏，不滚动，内部面板各自滚动
3. **面板怎么组织？** → slot 制，slot 是逻辑容器，不是固定 DOM
4. **布局可变吗？** → 用户可切 1/2/3 列，可拖拽 resize，可弹出浮窗
5. **谁拥有状态？** → 不是组件 state，是 `workspace_profile` 后端持久化

这些约束决定了后续所有技术选型。**如果你跳过这一步直接写组件，后面改布局架构的成本是推倒重来。**

### Step 1：搭最外层骨架（Desktop）

先不管里面放什么，先把"顶栏 + 主区 + 底栏"的垂直结构搭出来：

```
Desktop
  └─ div.flex.h-svh.flex-col.overflow-hidden
     ├─ <Menubar />          ← h-10 shrink-0
     ├─ <main flex-1>        ← min-h-0（关键：让内部能 shrink）
     │   └─ {children}      ← 路由页面
     └─ <Dock />             ← shrink-0
```

**验证标准**：浏览器打开，能看到上中下三栏占满视口，中间区可缩放不撑破。

**关键 CSS 决策**：
- 外层 `h-svh` 而非 `h-screen`：移动端浏览器地址栏伸缩时 `svh` 更稳定
- `main` 加 `min-h-0`：flex 子元素默认 `min-height: auto`，不加这个会导致内容溢出撑破布局
- `overflow-hidden` 在外层：禁止整页滚动，滚动交给内部面板

### Step 2：接入路由与 Provider

骨架能渲染后，接入路由让 `main` 里能切换页面：

```
App.tsx
  └─ Routes
     ├─ "/" → <Desktop><FleetPage /></Desktop>
     ├─ "/w/:workspaceId" → <Desktop><WorkspacePage /></Desktop>
     └─ "/login" → <LoginPage />（不套 Desktop）
```

同时套上 Provider 嵌套树（详见 [00 总体架构](./00-overview.md)）：

```
StrictMode > QueryClientProvider > ThemeProvider > BrowserRouter
  > AuthProvider > TooltipProvider > App
```

**验证标准**：路由切换时只有 `main` 内部重新渲染，Menubar 和 Dock 不闪。登录态切换（AuthProvider）能驱动页面重定向。

### Step 3：定义 slot 模型与布局切换

这是布局的**核心抽象**。不要急着写 UI，先把数据模型定下来：

```typescript
// SlotConfig：每个 slot 的身份与默认打开哪些 app
interface SlotConfig {
  id: string                    // 'slot-a' / 'slot-b' / 'slot-c'
  defaultOpened: string[]       // ['files', 'chat', ...]
}

// LayoutDef：一种布局的定义
interface LayoutDef {
  id: LayoutId                   // '1col' | '2col' | '3col'
  slots: SlotConfig[]           // 这个布局有几个 slot
  Component: ComponentType       // 渲染这个布局的 React 组件
  fallbackSlotId: string         // ensureInstance 时的兜底 slot
}
```

然后写 layout 组件。最简单的 `OneColumnLayout`：

```tsx
function OneColumnLayout() {
  return (
    <div className="flex h-full min-h-0 gap-0.5 p-3">
      <div className="flex min-w-0 flex-1">
        <SlotContainer slotId="slot-a" />
      </div>
    </div>
  )
}
```

**验证标准**：切 layoutId 时，slot 数量与排列方式正确变化。此时 SlotContainer 可以先返回占位文字。

### Step 4：实现 SlotContainer 的实例切换

SlotContainer 的核心逻辑：

```typescript
function SlotContainer({ slotId }) {
  const ctx = useRequiredSlotContext()
  const state = ctx.getState(slotId)
  const instance = state.active           // 当前激活实例
  if (!instance) return <SlotEmptyState />
  const app = ctx.getApp(instance.appId)  // 查 AppDefinition
  return <AppWindow><App key={instance.id} instanceId={instance.id} /></AppWindow>
}
```

**关键设计决策**：
- `key={instance.id}`：切实例时旧组件 unmount、新组件 mount。想跨切保留状态必须提到上层 store（如 chat 消息存在 agent-session-store，不在组件里）
- `SlotEmptyState`：slot 空时显示一个居中的 `+` 按钮，点开 SlotPicker

**验证标准**：Dock 里点不同 pill，对应 slot 的内容切换。同一个 slot 切不同实例，状态不串。

### Step 5：实现 AppWindow 的 chrome 与 portal header

AppWindow 提供：
- 外壳样式（border/radius/shadow/bg-card）
- header（h-9，左侧 portal target，右侧 WindowControls）
- body（flex-1，放 children）

```tsx
function AppWindow({ children, onClose, onPopOut, onFill, isFilled }) {
  const [headerSlot, setHeaderSlot] = useState(null)
  return (
    <section className="rounded-lg border bg-card shadow-xl flex flex-col h-full">
      <header className="flex h-9 items-center gap-2 px-2 border-b">
        <div ref={setHeaderSlot} className="flex flex-1" />  {/* portal target */}
        <WindowControls onClose={onPopOut} ... />
      </header>
      <AppHeaderSlotContext.Provider value={headerSlot}>
        <div className="flex-1">{children}</div>
      </AppHeaderSlotContext.Provider>
    </section>
  )
}
```

App 组件内部用 portal 注入 header 内容：

```tsx
function FilesApp() {
  const headerSlot = useAppHeaderSlot()
  return (
    <>
      {headerSlot && createPortal(<PathBreadcrumbs />, headerSlot)}
      <FileList />
    </>
  )
}
```

**为什么用 portal 而不是 props 传 header 内容**：因为 App 的 header 内容（路径输入框、选择器、操作按钮）依赖 App 内部状态，如果通过 props 往上提到 AppWindow 会打破 App 的自治边界。portal 让 App 自己管理 header 内容，但渲染位置由 shell 控制。

**验证标准**：不同 App 的 header 内容不同但位置统一。WindowControls 在 hover 时出现，不抢 App 内容。

### Step 6：实现 Dock 的 pill 列表与拖拽

Dock 是最复杂的 shell 组件（`Dock.tsx` 约 700 行）。拆成几个子问题：

1. **pill 列表**：每个 slot 一个 `DockSegment`，显示 `state.opened` 的实例
2. **激活切换**：点 pill 调 `ctx.activate(slotId, instanceId)`
3. **拖拽排序**：用 `@dnd-kit/sortable`，跨 slot 拖拽调 `ctx.moveInstance`
4. **容量裁剪**：视口不够时 `distributeCapacity` 收起最宽的 pill
5. **添加实例**：`SlotPicker` 弹出 App 列表，选了调 `ctx.open(slotId, appId)`

**验证标准**：打开 10 个 app，pill 不溢出屏幕。拖拽 pill 到别的 slot，实例跟着移动。

### Step 7：接入 PopoutLayer 浮窗

用户点 AppWindow 的"弹出"按钮，实例从 slot 移到虚拟的 `POPOUT_SLOT_ID`：

```
popOut(slotId, instanceId)
  ├─ 从 slot.opened 移除
  ├─ push 到 popout.opened
  └─ PopoutLayer 检测到 popout.opened 非空，渲染浮窗
```

PopoutLayer 自己提供 chrome（拖拽/resize/tab strip），不套 AppWindow。

**验证标准**：弹出后浮窗可拖拽、可 resize、可 popIn 回 slot。关掉浮窗实例回到原 slot。

### Step 8：持久化与恢复

最后一步：用户刷新页面后布局要恢复。这通过 `workspace-profile-store` 实现：

```
Desktop mount
  └─ ensureLoaded(profileId)
     └─ GET /workspaces/:id/profile
        └─ 返回 { layout_id, selected_layout_id, slots, instances, ... }
           └─ 写入 byWorkspace[profileId]
              └─ useWorkspaceProfileLoaded 返回 true
                 └─ slotConfig 计算出非 null
                    └─ SlotProvider 挂上，布局渲染
```

**关键**：`profileLoaded` 为 false 前，`slotConfig` 是 null，不挂 SlotProvider，只渲染 shell 骨架。避免闪烁——先闪默认布局再跳到用户保存的布局。

**验证标准**：调整布局后刷新，布局恢复。切换 workspace 再切回，布局各自保留。

## 四、从零规划一个类似前端项目的步骤指南

如果你要新起一个类似的多面板桌面布局项目，按这个顺序规划：

### 阶段 A：定义产品形态（1-2 天）

| 要回答的问题 | 选项 | 本项目的选择 |
|-------------|------|--------------|
| 应用形态 | 滚动网页 / 多面板桌面 / 单页应用 | 多面板桌面 |
| 视口策略 | 全屏不滚 / 纵向滚动 / 混合 | 全屏不滚（h-svh） |
| 面板模型 | 固定网格 / 可拖拽 slot / 自由窗口 | 可拖拽 slot + 浮窗 |
| 布局可变 | 固定 / 预设切换 / 完全自定义 | 预设切换（1/2/3 col） |
| 状态归属 | 组件 state / 全局 store / 后端 | 后端持久化（workspace_profile） |

**产物**：一张产品形态决策表。后面所有技术选型都基于这张表。

### 阶段 B：技术选型与脚手架（2-3 天）

| 维度 | 选型 | 理由 |
|------|------|------|
| 框架 | React 18 | hooks + Suspense + lazy 够用 |
| 状态管理 | Zustand（vanilla + Context） | 不需要 Redux 的仪式感，store 可脱 React 用 |
| 数据获取 | TanStack Query | 缓存/失效/重试开箱即用 |
| 路由 | React Router v6 | 嵌套路由 + lazy |
| 样式 | Tailwind CSS | 原子类，布局快 |
| 拖拽 | @dnd-kit | React 友好，支持跨容器排序 |
| UI 组件 | shadcn/ui + Radix | 可定制，不锁框架 |
| i18n | react-i18next | 标准选择 |

**脚手架顺序**：
1. `vite` 初始化 + Tailwind 配置
2. Provider 嵌套树先搭好（AuthProvider/QueryClientProvider/ThemeProvider/Router）
3. 最简路由能跑通（两个空页面互相跳）

### 阶段 C：定义核心数据模型（3-5 天）

**这是最重要的阶段**。数据模型定错了，后面 UI 全要返工。

```
需要定义的模型：
├─ AppDefinition    // 一个 App 是什么（id/label/Component/icon/...）
├─ AppInstance      // 一个 App 的运行实例是什么（id/appId）
├─ SlotConfig       // slot 的配置（id/defaultOpened）
├─ LayoutDef        // 布局定义（id/slots/Component/fallbackSlotId）
├─ SlotState        // slot 运行时状态（opened/active/activeId）
├─ WorkspaceProfile // 持久化的完整布局快照（layout_id/slots/instances/...）
└─ ProfileBackend   // 后端抽象（get/patch/subscribe）
```

每个模型问自己三个问题：
1. 它的**身份**是什么（什么字段唯一标识）？
2. 它**归谁管**（组件 / store / 后端）？
3. 它**何时变**（用户操作 / 数据推送 / 派生计算）？

### 阶段 D：搭骨架与占位（2-3 天）

按 Step 1-3 的顺序：Desktop 骨架 → 路由接入 → layout 组件占位。

**验证标准**：能切路由，能切布局，slot 里显示占位文字。**此时不碰任何业务逻辑。**

### 阶段 E：实现 SlotContext 核心（5-7 天）

这是项目的"心脏"。`SlotContext` 提供 14 个 API（详见 [03 Shell](./03-shell-and-chat.md)）：

| 分组 | API | 作用 |
|------|-----|------|
| 读 | `getState` / `getApp` / `slots` / `filledSlot` | 取 slot 状态 |
| 打开/关闭 | `open` / `close` / `activate` / `ensureInstance` | 管理 App 实例 |
| 移动 | `moveInstance` / `popOut` / `popIn` | 跨 slot / 弹出弹回 |
| 填充 | `setFilledSlot` | 全屏某个 slot |
| 配置 | `popoutGeo` / `setPopoutGeo` | 浮窗位置 |

**实现策略**：先写一个内存版（useState 管的 SlotState），跑通 UI 交互后再接 `workspace-profile-store` 持久化。

### 阶段 F：填充业务 App（每个 2-5 天）

骨架搭好后，按 App 一个个填：

1. 先做 **Chat App**（核心功能，验证 agent-session-store 能挂通）
2. 再做 **Files App**（验证 instance-state-store 的易失/持久分层）
3. 然后 **Sessions App**（验证与 active-session-store 的桥接）
4. 最后 **Skills / Memory / Settings / Terminal / Browser**（各有自己的数据源）

每个 App 的 checklist：
- [ ] 定义 AppDefinition 注册到 app-registry
- [ ] 实现 Component（主体内容）
- [ ] 用 portal 注入 header 内容
- [ ] 定义 instance state（易失/持久）
- [ ] 在 Dock 里能打开/关闭/激活

### 阶段 G：打磨交互（持续）

- Dock 的拖拽排序与容量裁剪
- 布局切换的动画过渡
- 浮窗的拖拽/resize/吸附
- 命令面板的搜索与快捷键
- 暗色/亮色/壁纸主题

## 五、布局相关的设计决策记录

把项目里几个"为什么这么设计"的决策记下来，新人不用重新推导：

### 决策 1：为什么 slot 状态不放在组件里？

**背景**：slot 的 opened/active 列表需要跨组件读（Dock 读 opened 渲染 pill，SlotContainer 读 active 渲染内容，SlotPicker 读 opened 判断是否已存在）。

**决策**：放在 `workspace-profile-store`（Zustand vanilla store），组件通过 `useWorkspaceProfile` selector 订阅切片。

**代价**：心智模型比 `useState` 复杂，但换来了"任何组件都能读 slot 状态，修改自动同步"。

### 决策 2：为什么 layout 是组件而不是配置？

**背景**：不同 layout 的 slot 数量、宽度策略、resize 手柄位置都不同。

**决策**：每种 layout 是一个独立组件（`OneColumnLayout` / `TwoColumnLayoutDefault` / `ThreeColumnLayout`），在 `LayoutDef.Component` 里注册。

**代价**：加一种 layout 要写一个组件，但换来了"每种 layout 的布局逻辑完全独立，互不干扰"。

### 决策 3：为什么 AppWindow 用 portal 而不是 props？

**背景**：App 的 header 内容（路径输入框、会话选择器）依赖 App 内部状态，但渲染位置在 AppWindow 的 header 里。

**决策**：AppWindow 暴露 `AppHeaderSlotContext`（一个 div ref），App 用 `createPortal` 注入内容。

**代价**：portal 有一定心智成本，但换来了"App 自治 header 内容，shell 统一控制 chrome"。

### 决策 4：为什么 profileLoaded 为 false 时不挂 SlotProvider？

**背景**：用户刷新页面后，需要先 `GET /workspaces/:id/profile` 拿到保存的布局，才能正确渲染 slot。

**决策**：`Desktop.tsx:75` 的 `slotConfig` 在 `profileLoaded` 为 false 时返回 null，不挂 SlotProvider，只渲染 shell 骨架。

**代价**：首屏多一个"空壳"瞬间，但避免了"先闪默认布局再跳到用户保存的布局"的闪烁问题。

### 决策 5：为什么 PopoutLayer 在 ws scope 挂在 WorkspacePage 里而不是 Desktop？

**背景**：弹出的 chat 浮窗需要读 agent-session-store 的数据。

**决策**：ws scope 下 PopoutLayer 挂在 `WorkspacePage` 的 `AgentSessionProvider` 内（`WorkspacePage.tsx:100`），确保浮窗能找到 session context。fleet scope 下没有 session context，挂在 Desktop 直接渲染。

**代价**：两处挂载点，但换来了"浮窗里的 chat app 能正常订阅 session 数据"。

## 六、快速定位口诀

当你看着屏幕不知道某块是谁渲染的时候，按这个顺序找：

```
屏幕上某块东西
  ├─ 在最顶部（h-10 横条）→ Menubar.tsx
  ├─ 在最底部（胶囊形横条）→ Dock.tsx
  ├─ 在中间且是窗口边框/标题栏 → AppWindow.tsx
  ├─ 在窗口标题栏左侧的内容 → App 自己的 portal（createPortal 到 useAppHeaderSlot()）
  ├─ 在窗口标题栏右侧的按钮 → AppWindow 的 WindowControls
  ├─ 在窗口内部主体 → App.Component
  ├─ 是浮动的窗口 → PopoutLayer.tsx
  ├─ 是 ⌘K 弹出的搜索面板 → CommandPalette.tsx
  └─ 是模态对话框 → DialogStackContext 的消费者
```

```
想知道某块受谁控制
  ├─ 布局（1/2/3 列）→ workspace_profile.layout_id（后端持久化）
  ├─ slot 打开了哪些 app → workspace_profile.slots（后端持久化）
  ├─ slot 当前激活哪个 → slot state.active（后端持久化）
  ├─ 浮窗位置/大小 → workspace_profile.popoutGeo（后端持久化）
  ├─ 实例内部状态（cwd/viewingPath）→ workspace_profile.instances（后端持久化）
  ├─ chat 消息/SSE → agent-session-store（内存，per-workspace 隔离）
  ├─ 当前会话 → active-session-store（内存，全局单例）
  └─ 面板宽度 → localStorage（useResizablePanel 的 storageKey）
```

## 延伸阅读

读完本章后，按需深入：

- 想理解 slot 的 14 个 API 详细用法 → 读 [03 Shell/Slot 模型与聊天渲染管线](./03-shell-and-chat.md)
- 想理解 App 注册与实例状态 → 读 [05 App Registry 与 Slot 装配链路](./05-slots-and-app-registry.md)
- 想理解 workspace_profile 持久化字段 → 读 [workspace-profile 字段参考](./workspace-profile.md)
- 想理解 boot 启动到第一个画面的完整时序 → 读 [04 端到端阅读路径](./04-reading-paths.md) 主线 1
- 想看具体 layout 组件实现 → 读 `shell/layout/OneColumnLayout.tsx` / `TwoColumnLayout.tsx` / `ThreeColumnLayout.tsx`
- 想看 AppWindow portal header 实现 → 读 `shell/windows/AppWindow.tsx:48-52`（context）+ `:100-124`（header + provider）
- 想看 Dock 拖拽与容量计算 → 读 `shell/Dock.tsx:68`（distributeCapacity）+ `:329`（DndContext）

**回到**：[INDEX](./INDEX.md) | [00 总体架构](./00-overview.md) | [03 Shell](./03-shell-and-chat.md) | [05 App Registry](./05-slots-and-app-registry.md)
