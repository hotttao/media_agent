# 05 App Registry 与 Slot 装配链路

> 分章。补 03 章没展开的两件事：(1) `lib/app-registry.ts` 定义的 App 类型，App 怎么从"组件定义"被装配成 dock 里的图标、再被点开成 slot 里的实例；(2) `contexts/SlotContext.tsx` 与 `stores/instance-state-store.ts` 之间的循环 import 为什么目前没爆，以及它在什么情况下会爆。读完你应该能解释一个 AppDefinition 是怎么流到屏幕上的，以及为什么 `useInstanceState` 不需要传 `workspaceId`。

## 代码地图

### 类型层

| 文件 | 行数 | 职责 |
|------|------|------|
| `lib/app-registry.ts` | 69 | **只定义类型**：`AppDefinition` / `AppComponentProps` / `AppInstance`。零运行时代码 |

### App 来源（装配层）

| 文件 | 职责 |
|------|------|
| `hooks/useWsApps.ts` | workspace scope 的 `AppDefinition[]` 构造器（257 行）。内置 app + MCP 插件 app |
| `hooks/useFleetApps.ts` | fleet scope 的 `AppDefinition[]` 构造器。user-scoped 资源浏览器（workspaces/tags/activity/library/...） |
| `components/shell/apps/wsApps.tsx` | 所有内置 App Component 实现 + `getPluginApp(pluginId)` 插件 App 工厂 |

### Slot 装配层

| 文件 | 职责 |
|------|------|
| `contexts/SlotContext.tsx` | SlotProvider + 14 API + POPOUT_SLOT_ID 虚拟槽 |
| `components/shell/Desktop.tsx` | 调 `useWsApps` / `useFleetApps` 拿 apps，传给 `SlotProvider` |
| `components/shell/Dock.tsx` | dock 列出 `ctx.apps`（不含 hidden） |
| `components/shell/SlotPicker.tsx` | 挑 app 打开到 slot 的 UI，按 `group` 分组 |
| `components/shell/SlotContainer.tsx` | 渲染单个 slot 的 active instance（`<App key={instance.id} />`） |

### per-instance 状态层

| 文件 | 职责 |
|------|------|
| `stores/instance-state-store.ts` | 两个 hook：`useInstanceState`（易失）+ `useInstancePersistentState`（持久化到 workspace_profile）。**这就是和 SlotContext 循环 import 的那一方** |
| `stores/workspace-profile-store.ts` | `workspace_profile` 的 zustand 单例。slot 状态 + 持久化 instance state 都住在这里 |

## 抓手拆解

### 抓手 1：AppDefinition 的 7 个字段

`lib/app-registry.ts:12-49` 是整个 App 体系的契约。逐字段看：

| 字段 | 类型 | 用途 |
|------|------|------|
| `id` | `string` | App 的稳定标识。所有 slot/profile 引用都用它，**不会变** |
| `label` | `string` | dock 显示名（i18n 后的字符串） |
| `Component` | `ComponentType<AppComponentProps>` | App 的 React 组件。**必须是 module-level 引用**，否则 apps 数组每次重生成会触发 SlotContainer remount |
| `disabled` | `boolean?` | dock 灰显、SlotPicker 隐藏。但已 active 的实例仍渲染（让它自己显示 NotRunning）。`useWsApps.ts:110` 的 `notRunning = ws?.status !== 'running'` 就是用这个 |
| `group` | `enum?` | SlotPicker 分组：`agent`/`tool`/`capability`/`connection`/`extension`/`system`。默认 `tool` |
| `hidden` | `boolean?` | 不进 dock 也不进 SlotPicker，只能 `openInPopout` 创建。`FileApp`（单文件查看器）就是 hidden，配 `openInPopout('file', { viewingPath: '/x.md' })` |
| `badge` | `string?` | dock 角标（如 `preview`）。`teamwork` app 用 |
| `instanceLabel` | `(persistent) => string?` | 多实例 tab 区分用。`FileApp` 用 `viewingPath` 的 basename |

注释（line 3-11）原话强调一个关键约束：

> Component must be a stable reference (module-level export). Per-instance state (cwd, scroll, expanded tree, draft) lives in the instance state store keyed by `instanceId`, NOT in component-local useState - that way slot moves / fill-mode toggles don't reset the user's view.

也就是说 App 组件不允许把状态放 `useState`，否则切 slot/fill 时会丢。这是整个 instance-state-store 体系存在的根本原因。

### 抓手 2：App 与 AppInstance 的二元关系

`lib/app-registry.ts:51-69` 定义两个相关类型：

```typescript
export interface AppComponentProps {
  /** Stable id assigned when the instance is created. */
  instanceId: string
}

export interface AppInstance {
  id: string       // 实例 id（每次 open 生成新的）
  appId: string    // 指回 AppDefinition.id
}
```

**关键区分**：
- `AppDefinition` = "这个 App 长什么样"（类）
- `AppInstance` = "这个 App 在某 slot 的一次实例化"（对象）

类比：`AppDefinition` 是组件函数，`AppInstance` 是 React Element。同一个 AppDefinition 可以实例化出多个 AppInstance（`id` 不同、`appId` 相同）。

实例化的产物落在哪里？看 `SlotContext.tsx:198-202` 的 `getState`：

```typescript
const seed: AppInstance[] = cfg?.defaultOpened?.map((appId) => ({
  id: `${slotId}:${appId}`,   // 确定性 id（未写过时复用）
  appId,
})) ?? []
```

和 `SlotContext.tsx:247-255` 的 `open`：

```typescript
const fresh: AppInstance = { id: newInstanceId(), appId }
writeSlot(slotId, { opened: [...cur, fresh], active: fresh.id })
```

两种 instance id 生成策略：
1. **defaultOpened 种子**：`${slotId}:${appId}`，确定性。为了让未写过 slot 的多次读返回同一 id，否则 React `key` 变化触发 remount
2. **用户新开**：`Math.random().toString(36).slice(2, 10)`（`SlotContext.tsx:153`）。不追求全局唯一，只追求不撞

### 抓手 3：App 的两种来源

`useWsApps` 装出来的 `AppDefinition[]` 是混合的：

**来源 A：内置 App**（`useWsApps.ts:80-216`）

```typescript
apps.push({
  id: 'chat',
  label: t('pages.workspace.tabs.chat'),
  Component: ChatApp,        // 从 components/shell/apps/wsApps.tsx import
  group: 'agent',
})
```

每个内置 App 在 `components/shell/apps/wsApps.tsx` 里有一个对应的导出（`ChatApp` / `FilesApp` / `TerminalApp` / ...）。这些 App 大多是薄壳，把 `instanceId` 透传给真正干活的 `WorkspaceXxxPanel`：

```typescript
export function FilesApp({ instanceId }: AppComponentProps) {
  const ws = useCurrentWorkspace()
  if (!ws) return null
  if (ws.status !== 'running') return <NotRunning />
  return (
    <Suspense fallback={<AppFallback />}>
      <WorkspaceFilesPanel workspaceId={ws.id} instanceId={instanceId} />
    </Suspense>
  )
}
```

注意三点：
1. App 组件自己不 fetch workspace，而是用 `useCurrentWorkspace()` 从路由 context 拿
2. 用 `Suspense` 包裹真正的 Panel，所以 Panel 可以 `lazy()` 加载
3. `instanceId` 透传给 Panel，Panel 用它做 per-instance state 的 key

**来源 B：插件 App**（`useWsApps.ts:218-244`）

```typescript
if (config?.mcp_config && catalog) {
  for (const entry of catalog) {
    if (!entry.ui_panel || !(entry.id in servers)) continue
    const panel = getPanel(entry.ui_panel)
    const lazyDesc = panel ? null : getLazyPanel(entry.ui_panel)
    const label = panel?.label() ?? lazyDesc?.label
    if (!label) continue
    apps.push({
      id: entry.id,
      label,
      Component: getPluginApp(entry.ui_panel),  // 工厂返回的包装组件
      group: 'extension',
    })
  }
}
```

关键：插件 App 的 `Component` 不是直接的 panel.component，而是 `getPluginApp(pluginId)` 返回的包装（`wsApps.tsx:425-461`）：

```typescript
export function getPluginApp(pluginId: string): ComponentType<AppComponentProps> {
  let cached = pluginAppCache.get(pluginId)
  if (cached) return cached
  function PluginApp({ instanceId }: AppComponentProps) {
    const ws = useCurrentWorkspace()
    useSyncExternalStore(subscribePanels, getPanelsVersion)
    useSyncExternalStore(subscribeLazyRegistry, getLazyRegistryVersion)
    const panel = getPanel(pluginId)
    const lazyDesc = panel ? null : getLazyPanel(pluginId)
    useEffect(() => {
      if (lazyDesc) ensurePluginLoaded(lazyDesc.pluginId).catch(() => {})
    }, [lazyDesc])
    if (!ws) return null
    if (!panel) return <Spinner />  // bundle 还没加载完
    const Comp = panel.component
    return <Comp workspaceId={ws.id} instanceId={instanceId} />
  }
  cached = PluginApp
  pluginAppCache.set(pluginId, cached)
  return cached
}
```

**为什么需要包装**：
- panel.component 在插件 bundle 加载完之前不存在
- 包装层用 `useSyncExternalStore` 订阅两个注册表，bundle 到了就重渲染
- 还没到就显示 Spinner 并 `ensurePluginLoaded` 触发加载
- module-level `pluginAppCache` 保证同一 pluginId 拿到同一 ComponentType，否则 apps 数组重生成会让 SlotContainer 误判 prop 变了而 remount

**三种 panel 状态**（注释 `useWsApps.ts:227-230`）：
1. **已注册**：bundle 加载完，`getPanel` 拿到，用 `panel.label()`（支持 i18n）
2. **lazy descriptor**：清单已到但 bundle 没加载，`getLazyPanel` 拿到占位描述符，用 `lazyDesc.label`（manifest 静态标签）。tab 先出现，等用户点再加载
3. **都没有**：跳过。等清单 fetch 完再进来

`useWsApps.ts:61-65` 的两个 `useSyncExternalStore` 就是让 hook 在 panel/lazy registry 变化时重算 apps 数组：

```typescript
const panelsVersion = useSyncExternalStore(subscribePanels, getPanelsVersion)
const lazyVersion = useSyncExternalStore(subscribeLazyRegistry, getLazyRegistryVersion)
```

注释（line 67-69）：

> panelsVersion is intentionally a dep - it isn't read in the body but bumps when a plugin panel registers late, forcing the app list to recompute.

### 抓手 4：App 的 group 分类

`lib/app-registry.ts:21-29` 定义 6 个 group：

| group | 心智模型 | ws 例子 | fleet 例子 |
|-------|----------|---------|-----------|
| `agent` | agent 本身 | chat, sessions, memory, settings | workspaces, tags, activity, teamwork |
| `tool` | 用户直接操作的工作面 | files, terminal, browser, sandboxes | - |
| `capability` | 扩展 agent 能力 | skills, automation, library | library, memory-stores |
| `connection` | 桥接外部系统 | connectors, service-tokens, credentials | connectors, service-tokens, credentials, environments |
| `extension` | 用户装的插件 app | MCP-driven panel | - |
| `system` | 平台级管理 | models, teams, oauth-apps, admin | models, teams, oauth-apps, admin |

注释强调 group 按"用户心智"分，不按实现 scope：

> Categories follow user mental model rather than implementation scope

`SlotPicker` 用 `GROUP_ORDER` 排序分组渲染。`useWsApps.ts:76-78`：

> Push order within a group is preserved by SlotPicker, so put each app where the user expects to find it in its column. Cross-group order doesn't matter for the picker (GROUP_ORDER takes over there).

所以 push 顺序只在组内有意义。

### 抓手 5：useWsApps / useFleetApps 的装配时序

`Desktop.tsx:62-63` 调两个 hook 拿 apps：

```typescript
const wsApps = useWsApps(workspaceId)
const fleetApps = useFleetApps()
```

然后 `Desktop.tsx:75-91` 装进 slotConfig：

```typescript
const slotConfig = useMemo(() => {
  if (!profileLoaded) return null
  if (scope === 'ws' && workspaceId) {
    return { slots: LAYOUTS[layoutId].slots, apps: wsApps, id: workspaceId, fallbackSlotId }
  }
  if (scope === 'fleet') {
    return { slots, apps: fleetApps, id: FLEET_PROFILE_ID, fallbackSlotId }
  }
  return null
}, [...])
```

`SlotProvider` props 接收 apps（`SlotContext.tsx:177`）：

```typescript
export function SlotProvider({ workspaceId, slots: layoutSlots, apps, fallbackSlotId, children })
```

然后 `SlotContext.tsx:188`：

```typescript
const getApp = useCallback((id: string) => apps.find((a) => a.id === id), [apps])
```

这是 App 从 hook 流到 SlotContext 的完整路径：

```
useWsApps(wsId) ──┐
                  ├─► Desktop slotConfig.apps ─► SlotProvider apps ─► ctx.getApp(appId)
useFleetApps() ───┘                                                          │
                                                                             │
                                                              SlotContainer 渲染时用
                                                              `ctx.getApp(instance.appId)`
                                                              拿 AppDefinition.Component
```

**关键约束**：`apps` 数组在每次 `useWsApps` 重算时是**新数组**，但 `getApp` 用 `useCallback([apps])` 包了，所以只要 `apps` 引用变，`getApp` 引用也变，订阅 `getApp` 的消费者重渲染。如果 AppDefinition.Component 引用不稳（比如 inline 函数），SlotContainer 会因 `app.Component` 变化触发不必要的 remount。这就是 `getPluginApp` 用 `pluginAppCache` 缓存的原因。

### 抓手 6：App 进入 slot 的三条路径

| 路径 | 入口 | 何时用 |
|------|------|--------|
| **defaultOpened 种子** | `SlotConfig.defaultOpened` 配置 | 第一次访问 slot、profile 没存任何东西时。seed instance id 是 `${slotId}:${appId}` |
| **dock 点击** | `ctx.ensureInstance(appId)` | 用户主动开。已开则跳转，未开则按 defaultOpened/fallback 选 slot 新建 |
| **openInPopout 直接种** | `ctx.openInPopout(appId, seedPersistent)` | 程序化开。如 markdown 链接点文件路径，直接开 FileApp 在浮动层 |

`ensureInstance`（`SlotContext.tsx:264-283`）的解析顺序：

1. 遍历 layoutSlots，找已有实例 → 跳转（**跳过 popout**，注释 line 69-72）
2. 找 `defaultOpened` 包含这个 appId 的 slot → preferred
3. fallback = `fallbackSlotId` 配置的 slot → 第一格
4. 都没有 → `throw`

注释 line 273-277：

> Resolution order: defaultOpened match -> layout-aware fallback (work slot, set by Desktop) -> first slot. The fallback only kicks in for apps not anchored anywhere - chat / sessions stay on their column.

`fallbackSlotId` 是 Desktop 通过 `getFallbackSlotId(layoutId)`（`Desktop.tsx:77`）算出来的工作面 slot，永远不是 chat/sidecar 列。

### 抓手 7：per-instance state 的两层

App 组件不能把状态放 `useState`（抓手 1 已说）。替代方案在 `stores/instance-state-store.ts`，提供两个 hook：

| hook | 持久性 | 后端 | 用途 |
|------|--------|------|------|
| `useInstanceState` | 易失 | 无（zustand 内存） | 刷新即丢。适合 cheap-to-recompute：搜索框、加载的列表、展开状态 |
| `useInstancePersistentState` | 持久 | `workspace_profile.instances[instanceId][key]`（400ms 去抖 PATCH） | 刷新留存、跨设备。适合 cwd/drive/sort/view mode |

两者 API 完全镜像：

```typescript
const [cwd, setCwd] = useInstancePersistentState(instanceId, 'cwd', () => '/')
const [search, setSearch] = useInstanceState(instanceId, 'search', () => '')
```

三层 keying：`(workspaceId, instanceId, key)`。注释（line 26-32）解释为什么要 workspace 维度：

> slot default instance ids (e.g. `slot-a:settings`) are deterministic and therefore identical across workspaces; without the workspace dimension, a draft written by ws A's Settings would leak into ws B's Settings on switch.

也就是说 deterministic seed id 跨 workspace 撞车，必须再加 workspaceId 隔离。

**workspaceId 从哪来**（`instance-state-store.ts:100-102`）：

```typescript
const slotCtx = useSlotContext()
const { workspaceId: paramWs } = useParams<{ workspaceId?: string }>()
const workspaceId = slotCtx?.workspaceId ?? paramWs ?? ''
```

**这就是循环 import 的根源**：instance-state-store 想从 SlotContext 拿 workspaceId。下个抓手细说。

### 抓手 8：循环 import 分析（用户问的）

两个文件互相 import：

```
contexts/SlotContext.tsx
   │  imports
   ▼
dropInstanceState, dropPersistentInstanceState, setPersistentInstanceStateMany
   │  from @/stores/instance-state-store
   │
stores/instance-state-store.ts
   │  imports
   ▼
useSlotContext from @/contexts/SlotContext
```

#### 双方各用了对方的什么

**SlotContext 用 instance-state-store 的**（`SlotContext.tsx:1-6`）：

```typescript
import {
  dropInstanceState,
  dropPersistentInstanceState,
  setPersistentInstanceStateMany,
} from '@/stores/instance-state-store'
```

3 个函数值导入。使用位置：
- `close`（line 307-308）：关实例时清两层 state
- `openInPopout`（line 397）：种 persistent state 给新 popout 实例

**instance-state-store 用 SlotContext 的**（`instance-state-store.ts:1`）：

```typescript
import { useSlotContext } from '@/contexts/SlotContext'
```

1 个 hook 值导入。使用位置：
- `useInstanceState`（line 100）：拿 workspaceId
- `useInstancePersistentState`（line 163）：拿 workspaceId

#### 为什么目前没爆

ESM 的循环 import 处理规则：
1. 假设 bundler 先开始解析 `SlotContext.tsx`
2. 遇到 `from '@/stores/instance-state-store'`，开始解析 instance-state-store
3. instance-state-store 遇到 `from '@/contexts/SlotContext'`，但 SlotContext 还没执行完
4. ESM 给 instance-state-store 的是 SlotContext 模块的 **live binding 引用**（此时该模块的 exports 还是空对象，但引用本身有效）
5. instance-state-store 继续执行：`useInstanceState` 和 `useInstancePersistentState` 是函数声明，定义时不调用，只是注册到模块作用域
6. instance-state-store 执行完，导出这两个函数
7. 回到 SlotContext 继续执行，import 拿到 `dropInstanceState` 等，模块顶层不立即用，只是注册
8. SlotContext 执行完，导出 `useSlotContext`、`SlotProvider` 等

**关键**：双方都只在**函数体内**使用对方的 import，不在模块顶层立即消费。运行时调用 `useInstanceState()` 时，SlotContext 模块早已完整执行完，`useSlotContext` 已经 export 出来，能正确解析。

这就是为什么 "目前不会爆"：使用是延迟的（lazy），不是即时的（eager）。

#### 什么情况下会爆

如果有人在任一文件**模块顶层**立即使用对方的导出，就会触发 `ReferenceError: Cannot access 'X' before initialization`。例如下面这种修改会让它爆：

```typescript
// ❌ 如果 instance-state-store.ts 顶层这样写
import { useSlotContext } from '@/contexts/SlotContext'
const DEFAULT_WS = useSlotContext()  // 模块顶层立即调用，SlotContext 还没执行完
```

或者：

```typescript
// ❌ 如果 SlotContext.tsx 顶层这样写
import { dropInstanceState } from '@/stores/instance-state-store'
dropInstanceState('default', 'init')  // 模块顶层立即调用
```

Vite/esbuild/webpack 这些现代 bundler 都能处理"延迟使用"型的循环 import，但运行时行为依模块求值顺序而定，不稳健。

#### 为什么设计成这样

两个反向需求撞在一起：

1. **SlotContext.close 需要清理 instance state**。理由：关掉的实例不能留着 draft 状态等下次复用（实例 id 已经丢了，留着是垃圾）。
2. **useInstanceState 需要知道 workspaceId**。理由：deterministic seed id 跨 workspace 撞车，必须隔离。

需求 1 让 SlotContext 依赖 instance-state-store。需求 2 让 instance-state-store 依赖 SlotContext。所以循环。

#### 这是设计问题吗

**是**。虽然有运行时不爆，但：
- 增加阅读成本，新人看到会困惑（用户的问题就是证据）
- 脆弱：未来有人在模块顶层消费对方导出就爆
- 工具链警告：某些 linter/bundler 配置会报 warning

#### 三个改进方案

**方案 A（最小侵入）**：让 hook 接受可选 workspaceId

```typescript
export function useInstanceState<T>(
  instanceId: string,
  key: string,
  defaultFn: () => T,
  workspaceId?: string,  // 新增
): [T, (next: T | ((prev: T) => T)) => void] {
  const slotCtx = useSlotContext()
  const { workspaceId: paramWs } = useParams()
  const wsId = workspaceId ?? slotCtx?.workspaceId ?? paramWs ?? ''
  // ...
}
```

调用方在 SlotContext 外面用的时候显式传。SlotContext 内部的调用方（实际上没有）保持不传。这样 instance-state-store 不强制依赖 SlotContext，但保持向后兼容。

**方案 B（中等侵入）**：把 workspaceId 提到独立 Context

```typescript
// stores/workspace-id-context.tsx（新文件）
export const WorkspaceIdContext = createContext<string>('')

// SlotProvider 内部挂一层
<WorkspaceIdContext.Provider value={workspaceId}>
  {children}
</WorkspaceIdContext.Provider>

// instance-state-store.ts 改成
import { WorkspaceIdContext } from '@/stores/workspace-id-context'
const workspaceId = useContext(WorkspaceIdContext) ?? paramWs ?? ''
```

WorkspaceIdContext 是叶子 context，不依赖 instance-state-store，所以无循环。SlotContext 内部挂它，对外 API 不变。

**方案 C（彻底重构）**：把 `dropInstanceState` / `setPersistentInstanceStateMany` 等命令式 API 提到独立模块

把 instance-state-store 拆成两个文件：
- `instance-state-store.ts`：只放 zustand store 本体 + `useInstanceState` / `useInstancePersistentState`（hooks）
- `instance-state-actions.ts`：放 `dropInstanceState` / `dropPersistentInstanceState` / `setPersistentInstanceState` / `setPersistentInstanceStateMany`（命令式 API）

SlotContext 只 import actions 文件，actions 文件不 import SlotContext。循环消失。

三个方案取舍：
- A 最快但留下"可选参数"的尾巴
- B 最干净但要新增 Context Provider
- C 改动最大但分层最清晰

如果只想止损（"它现在能跑就别动"），保持现状也行，但要在文件顶部加注释说明循环依赖的约束（"双方都不得在模块顶层立即使用对方导出"）。

## 端到端时序：一个 App 从注册到关闭

把所有抓手串起来。场景：用户首次进入 `/w/ws-1`，profile 已加载，defaultOpened 在 slot-a 种了 chat app；用户在 dock 点 Files 开新实例；最后关掉它。

```
═══ 阶段 A：App 注册（profile 加载前已经完成）═══

1. main.tsx boot
   └─ registerBuiltinPlugins()   // 不注册 app，app 不走插件注册表

2. Desktop.tsx mount
   └─ useWsApps('ws-1')
       ├─ push ChatApp/SessionsApp/.../FileApp  (内置)
       └─ if catalog+mcp_config: 遍历 MCP entry
           └─ getPluginApp(entry.ui_panel) 返回包装组件
       └─ return AppDefinition[]（含 FileApp，hidden=true）

3. Desktop.tsx slotConfig = { apps: wsApps, slots: LAYOUTS[layoutId].slots, ... }
   └─ profileLoaded=true 后挂 <SlotProvider apps={wsApps} slots={...}>

═══ 阶段 B：defaultOpened 种子实例化（首次访问）═══

4. SlotContainer 渲染 slot-a
   └─ ctx.getState('slot-a')
       ├─ storedSlots['slot-a'] = undefined（首次）
       ├─ seed = cfg.defaultOpened.map(appId => ({ id: `slot-a:${appId}`, appId }))
       │   └─ e.g. { id: 'slot-a:chat', appId: 'chat' }
       └─ activeId = opened[0].id = 'slot-a:chat'

5. SlotContainer 拿到 active instance
   └─ app = ctx.getApp('chat') -> AppDefinition
   └─ <AppWindow><ChatApp key="slot-a:chat" instanceId="slot-a:chat" /></AppWindow>
       └─ ChatApp 不需要 useInstanceState（它读 useAgentSessionStore）

═══ 阶段 C：用户点 dock 的 Files 图标═══

6. Dock.tsx 渲染图标
   └─ ctx.apps.filter(a => !a.hidden)  // FileApp 被过滤掉
   └─ 用户点 'files' 图标
       └─ ctx.ensureInstance('files')
           ├─ for slot in layoutSlots: 查 'files' 实例 → 无
           ├─ preferred = layoutSlots.find(s => s.defaultOpened?.includes('files'))
           │   └─ 假设 2col layout，slot-a 配 ['chat']，slot-b 配 []
           │   └─ preferred = undefined
           ├─ fallback = fallbackSlotId ?? layoutSlots[0]
           │   └─ Desktop 算出的 fallbackSlotId = 'slot-a'
           └─ open('slot-a', 'files')
               ├─ fresh = { id: 'ab12cd34' (random), appId: 'files' }
               └─ writeSlot('slot-a', { opened: [...prev, fresh], active: 'ab12cd34' })
                   └─ workspace-profile-store.patch('ws-1', { slots: ... })
                       └─ 400ms 去抖 PATCH /workspaces/ws-1/profile

7. workspace-profile-store 通知订阅者
   └─ useWorkspaceProfile('ws-1') 返回新 payload
       └─ SlotProvider 重渲染 -> SlotContainer 重渲染
           └─ state.active = { id: 'ab12cd34', appId: 'files' }
           └─ <AppWindow><FilesApp key="ab12cd34" instanceId="ab12cd34" /></AppWindow>

8. FilesApp mount
   └─ useCurrentWorkspace() -> ws
   └─ ws.status === 'running' ? <WorkspaceFilesPanel .../> : <NotRunning/>
   └─ WorkspaceFilesPanel 内部：
       ├─ const [cwd, setCwd] = useInstancePersistentState('ab12cd34', 'cwd', () => '/')
       │   └─ workspaceId 从 SlotContext.workspaceId 拿（'ws-1'）
       │   └─ 读 workspace_profile.instances['ab12cd34'].cwd → undefined → defaultFn()='/'
       └─ const [search, setSearch] = useInstanceState('ab12cd34', 'search', () => '')
           └─ workspaceId 同上
           └─ 读 byWorkspace['ws-1']['ab12cd34'].search → undefined → 默认 ''

═══ 阶段 D：用户切到 chat tab 再切回来，FilesApp 状态不丢═══

9. 用户点 slot-a 的 chat tab
   └─ ctx.activate('slot-a', 'slot-a:chat')
       └─ writeSlot('slot-a', { active: 'slot-a:chat' })
           └─ SlotContainer 重渲染，active 变 chat
               └─ <ChatApp key="slot-a:chat" ... /> 挂载
               └─ FilesApp 因 key 不再匹配 → unmount
                   └─ 但它的 cwd 在 workspace_profile.instances['ab12cd34'].cwd 里
                   └─ search 在 byWorkspace['ws-1']['ab12cd34'].search 里
                   └─ 两者都活

10. 用户点 Files tab 回来
    └─ ctx.activate('slot-a', 'ab12cd34')
        └─ writeSlot('slot-a', { active: 'ab12cd34' })
            └─ FilesApp 重新 mount
                └─ useInstancePersistentState('ab12cd34', 'cwd', () => '/')
                    └─ 读 profile → '/foo/bar'（之前切走时存的）
                └─ useInstanceState('ab12cd34', 'search', () => '')
                    └─ 读 byWorkspace → 'prev query'（如果切走时间 < 页面刷新）
                    └─ 刷新了则是 ''（in-memory 层丢）

═══ 阶段 E：用户关掉 Files 实例═══

11. 用户点 AppWindow 关闭按钮
    └─ ctx.close('slot-a', 'ab12cd34')
        ├─ nextOpened = opened.filter(i => i.id !== 'ab12cd34')
        ├─ nextActive = opened[idx-1].id = 'slot-a:chat'  (close-rule)
        └─ writeSlot('slot-a', { opened: nextOpened, active: nextActive })
        └─ dropInstanceState('ws-1', 'ab12cd34')
            └─ useStore.dropInstance('ws-1', 'ab12cd34')
                └─ delete byWorkspace['ws-1']['ab12cd34']
                    └─ search 等 in-memory 状态被清理
        └─ dropPersistentInstanceState('ws-1', 'ab12cd34')
            └─ workspace-profile-store.patch('ws-1', { instances: next })
                └─ 400ms 去抖 PATCH，cwd 等持久化状态从后端清掉

12. SlotContainer 重渲染
    └─ active 变 'slot-a:chat' → <ChatApp key="slot-a:chat" /> 重新挂载
    └─ FilesApp 实例彻底消失，所有状态清空，instance id 'ab12cd34' 不再被任何东西引用
```

整个链路的关键设计点：

- **AppDefinition 是稳定的**（module-level export + pluginAppCache），所以 apps 数组重生成不触发 remount
- **AppInstance 是动态的**，每次开/关都新生成 id
- **state 跟 instanceId 走，不跟组件走**：组件 unmount 但 state 还在 store 里，等回来
- **close 是单向的**：instance id 一旦 drop 就不可恢复（再开会生成新 id），所以 close 时必须清两层 state

## 延伸阅读建议

读完本章后：

- 想看 AppDefinition 字段权威定义 -> 读 `lib/app-registry.ts`（69 行，最短的一章）
- 想看所有内置 App 实现 -> 读 `components/shell/apps/wsApps.tsx`，每个 App 都是 5-10 行的薄壳
- 想看插件 App 工厂细节 -> 读 `wsApps.tsx:425-461` 的 `getPluginApp`
- 想看 SlotContext 14 API 全表 -> 回 [03 章](./03-shell-and-chat.md) 的"抓手 4"
- 想看 instance-state-store 完整实现 -> 读 `stores/instance-state-store.ts`（262 行，注释极详细）
- 想看 workspace-profile-store 的 PATCH 去抖 -> 读 `stores/workspace-profile-store.ts`

下一章 [04 端到端阅读路径](./04-reading-paths.md) 主线 3 "用户开一个新 App" 与本章阶段 C 是同一件事的两种讲法，可对照阅读。
