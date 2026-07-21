# workspace-profile 字段参考

> 分章。讲清 `WorkspaceProfilePayload` 这块 jsonb 里到底装了什么、每个字段谁在写谁在读、ws scope 和 fleet scope 的字段差异、以及 400ms 去抖 PATCH + 浅合并的持久化语义。读完你应该能回答："profile 里都有什么状态？我加一个新字段要改哪里？为什么 wallpaper 是 fleet 级而 cwd 是 ws 级？"

## 为什么是 jsonb 自由结构

`WorkspaceProfilePayload` 在 `lib/api/types.ts:400` 是从 `@neutree-ai/types` re-export 的：

```typescript
export type { WorkspaceProfilePayload } from '@neutree-ai/types'
```

实际类型是宽松的 `Record<string, unknown>`（jsonb 自由结构），后端不校验字段、不解析语义。设计意图见 `docs/wiki/control-plane/src/routes/workspaces/profile.md`：

- **客户端自管** - 任意 jsonb 结构，CP 不解析字段
- **不校验** - CP 只负责存取和浅合并
- **不触发 reload** - UI 偏好不影响 agent

所以字段清单**只在 web 端定义**，后端永远只是 jsonb 黑盒。本仓库的所有字段都通过 `as { xxx?: T }` 类型断言从 payload 取出，不依赖运行时 schema 校验。新增字段不需要后端 migration，只需要在前端加一个 hook + 一个类型断言。

## 代码地图

### Store 层

| 文件 | 职责 |
|------|------|
| `stores/workspace-profile-store.ts` | zustand 单例。`ensureLoaded` / `patch` / `useWorkspaceProfile` / `useWorkspaceProfileLoaded` + 400ms 去抖 flush |
| `stores/fleet-profile.ts` | `FLEET_PROFILE_ID = '__fleet__'` + `fleetBackend`（把 fleet profile 路由到 `user_profile` 表）+ 一次性 LS 迁移 |
| `stores/instance-state-store.ts` | 持久化 instance state 通过 `patch(wsId, { instances: ... })` 写入 profile |

### Hook 层（字段消费者）

| 文件 | 字段 | 用途 |
|------|------|------|
| `hooks/useActiveLayout.ts` | `layout_id` | 读写当前活动内置布局 |
| `hooks/useLayoutState.ts` | `selected_layout_id` / `instances` | layout FSM（real vs selected）+ re-stamp 清空 instance state |
| `hooks/useWallpaper.ts` | `wallpaper` | fleet 级壁纸预设 |
| `hooks/useChatSendKey.ts` | `chatSendKey` | fleet 级发送键偏好 |
| `contexts/SlotContext.tsx` | `slots` / `popoutGeo` | slot 状态 + popout 浮动窗几何 |

### 类型层

| 文件 | 职责 |
|------|------|
| `lib/api/types.ts:400` | re-export `WorkspaceProfilePayload`（jsonb 自由结构） |
| `@neutree-ai/types` | 真正的类型源（外部包）+ `skeletonToProfilePatch` 把 LayoutSkeleton 转 patch |
| `stores/workspace-profile-store.ts:13-17` | 仓库内唯一直接列字段清单的注释（但只列了 `layout_id` + `slots`，已过时） |

## 字段清单

7 个已知字段。按层级分组：

### ws + fleet 都用

| 字段 | 类型 | 默认 | 用途 |
|------|------|------|------|
| `layout_id` | `'1col' \| '2col' \| '3col'` | ws: `'3col'`<br>fleet: `'2col'` | 当前活动内置布局 id（`hooks/useActiveLayout.ts`、`hooks/useLayoutState.ts`） |
| `slots` | `Record<slotId, { opened?: AppInstance[], active?: string \| null }>` | layout 的 `defaultOpened` 种子 | 每个 slot 的实例列表与 active id（`contexts/SlotContext.tsx`） |
| `instances` | `Record<instanceId, Record<key, unknown>>` | `{}` | per-AppInstance 持久化 UI 状态（`stores/instance-state-store.ts`） |
| `popoutGeo` | `{ x: number, y: number, w: number, h: number } \| null` | `null`（PopoutLayer 用内置 `DEFAULT_W`/`DEFAULT_H`，位置 `-1` 居中） | 浮动弹出窗的几何（`contexts/SlotContext.tsx` 写、`components/shell/PopoutLayer.tsx` 读） |

### fleet only

| 字段 | 类型 | 默认 | 用途 |
|------|------|------|------|
| `selected_layout_id` | `string \| null` | `null` | 用户选中的自定义布局 db id（ws scope 才有自定义布局，fleet 不用） |
| `wallpaper` | `'aurora' \| 'minimal' \| 'cool' \| 'warm'` | `'aurora'` | 桌面壁纸预设（`hooks/useWallpaper.ts`，只写 `FLEET_PROFILE_ID`） |
| `chatSendKey` | `'mod-enter' \| 'enter'` | `'mod-enter'` | 聊天发送键偏好（`hooks/useChatSendKey.ts`，只写 `FLEET_PROFILE_ID`） |

> **关于 `selected_layout_id`**：从 `useLayoutState.ts:94` 看类型是 `string | null`，代码注释里 fleet 没有自定义布局概念，实际只会在 ws scope 被写。但字段本身没强制 fleet 不能写，只是没有 hook 触发。

## 抓手拆解

### 抓手 1：ProfileBackend 双后端（ws vs fleet）

`workspace-profile-store.ts:36-54` 的 `ProfileBackend` 抽象让 store 不关心后端在哪：

```typescript
export interface ProfileBackend {
  load: (id: string) => Promise<WorkspaceProfilePayload>
  save: (id, patch) => Promise<WorkspaceProfilePayload>
}

const apiBackend: ProfileBackend = {
  load: (id) => api.getWorkspaceProfile(id),       // 默认走 workspace_profile 表
  save: (id, patch) => api.patchWorkspaceProfile(id, patch),
}

const backendOverrides = new Map<string, ProfileBackend>()

export function registerProfileBackend(id: string, backend: ProfileBackend): void {
  backendOverrides.set(id, backend)
}

function backendFor(id: string): ProfileBackend {
  return backendOverrides.get(id) ?? apiBackend
}
```

两个后端的差异：

| profileId | 后端表 | HTTP | 谁注册 |
|-----------|--------|------|--------|
| 真实 workspaceId | `workspace_profile` | `GET/PATCH /api/workspaces/:id/profile` | 默认（`apiBackend`） |
| `FLEET_PROFILE_ID = '__fleet__'` | `user_profile` | `GET/PATCH /api/me/profile` | `fleet-profile.ts:65` `registerProfileBackend` |

`fleet-profile.ts:39-63` 的 `fleetBackend`：

```typescript
const fleetBackend: ProfileBackend = {
  load: async () => {
    const remote = await api.getUserProfile()
    // One-shot migration: if server is empty and there's pre-existing LS
    // payload, push it server-side and forget the LS key.
    const isEmpty = !remote || Object.keys(remote).length === 0
    if (isEmpty) {
      const legacy = readLegacyLs()
      if (legacy && Object.keys(legacy).length > 0) {
        const merged = await api.patchUserProfile(legacy)
        clearLegacyLs()
        return merged
      }
      clearLegacyLs()
    }
    return remote
  },
  save: async (_id, patch) => api.patchUserProfile(patch),
}

registerProfileBackend(FLEET_PROFILE_ID, fleetBackend)
```

**关键设计**：fleet profile 复用同一套 store + SlotContext + instance-state-store 机制，所以 fleet 也有 `slots` / `popoutGeo` / `instances` 字段。区别只在后端表名（`user_profile` vs `workspace_profile`）和路由前缀（`/api/me` vs `/api/workspaces/:id`）。

**LS 迁移一次性**：`fleet-profile.ts:17-37` 的 `LEGACY_LS_KEY = 'tos.fleet-profile.v1'` 是 pre-DB 时代的本地存储。首次加载时如果服务器为空且本地有 LS 数据，会推到服务器并删除 LS key。`MIGRATION_FLAG_KEY` 用 sessionStorage 标记"已迁移过"，避免每次都尝试读 LS。

### 抓手 2：layout_id vs selected_layout_id

两个字段都跟布局有关，但语义不同。`useLayoutState.ts:79-104` 的 FSM 是关键：

```typescript
const layoutId = (
  typeof profile.layout_id === 'string' ? profile.layout_id : DEFAULT_LAYOUT
) as LayoutId

const real = useMemo(() => {  // 当前 live 排布
  const slots: Record<string, string[]> = {}
  for (const slot of ctx.slots) {
    if (slot.id === POPOUT_SLOT_ID) continue
    const apps = ctx.getState(slot.id).opened.map((i) => i.appId)
    if (apps.length > 0) slots[slot.id] = apps
  }
  return normalizeLayoutSkeleton({ layout_id: layoutId, slots })
}, [ctx.slots, ctx.getState, layoutId])

const selectedId =
  typeof profile.selected_layout_id === 'string' ? profile.selected_layout_id : null
const selectedLayout = selectedId ? (layouts.find((l) => l.id === selectedId) ?? null) : null
const selected = selectedLayout
  ? normalizeLayoutSkeleton(selectedLayout.skeleton)
  : builtinSkeletonFor(layoutId)
const kind: LayoutKind = !selectedLayout
  ? 'builtin'
  : selectedLayout.origin === 'template' ? 'template' : 'custom'
const state: LayoutSyncState = layoutSkeletonEqual(real, selected) ? 'same' : 'edited'
```

字段对比：

| 字段 | 含义 | 取值 | 何时写 |
|------|------|------|--------|
| `layout_id` | **列布局 id**（决定 1/2/3 列框架） | `'1col' \| '2col' \| '3col'` | `useActiveLayout.ts:40` `setLayoutId`，用户切列布局时 |
| `selected_layout_id` | **选中的自定义/模板布局 db id** | db uuid 或 `null` | `useLayoutState.ts:109, 132` 的 `stamp` / `saveAsNew` |

`null` 表示"用内置默认（builtinSkeletonFor(layoutId)）"。`builtinSkeletonFor` 是缓存好的内置预设（`useLayoutState.ts:25-38`），每个 LayoutId 对应一个固定 skeleton。

`stamp` 函数（`useLayoutState.ts:106-116`）是 re-stamp 的关键：

```typescript
const stamp = useCallback(
  (skeleton: LayoutSkeleton, selected_layout_id: string | null) => {
    useWorkspaceProfileStore.getState().patch(workspaceId, {
      selected_layout_id,
      // Skeleton-only reuse: drop per-instance accumulated state on (re)stamp.
      instances: {},
      ...skeletonToProfilePatch(normalizeLayoutSkeleton(skeleton), mkInstanceId),
    })
  },
  [workspaceId],
)
```

re-stamp 时做三件事：
1. 写 `selected_layout_id`（null 表示回到内置默认）
2. **清空 `instances`**：注释明确说"Skeleton-only reuse: drop per-instance accumulated state"。因为新布局的 instance id 是新生成的，旧 instance state 是孤儿
3. `skeletonToProfilePatch(skeleton, mkInstanceId)` 生成 `slots` + 种子 `instances`（用 mkInstanceId 生成确定性 id）

`mkInstanceId`（`useLayoutState.ts:40-41`）：

```typescript
const mkInstanceId = (slotId: string, appId: string, i: number) =>
  `${slotId}:${appId}:${i}:${crypto.randomUUID().slice(0, 8)}`
```

注意：和 `SlotContext.tsx:199-202` 的种子 id `${slotId}:${appId}` 不同。re-stamp 的 id 带随机 UUID 后缀，是因为 stamp 是"应用布局"的一次性动作，需要新 id；而 SlotContext 的种子 id 是"未写过 slot 时的惰性合成"，需要确定性。

### 抓手 3：slots 字段的 shape

`SlotContext.tsx:147-151` 定义 `StoredSlot`：

```typescript
interface StoredSlot {
  opened?: AppInstance[]
  active?: string | null
}
type StoredSlots = Record<string, StoredSlot>
```

存储示例（2col 布局，slot-a 有 chat + files，slot-b 空）：

```json
{
  "slots": {
    "slot-a": {
      "opened": [
        { "id": "slot-a:chat", "appId": "chat" },
        { "id": "ab12cd34", "appId": "files" }
      ],
      "active": "ab12cd34"
    },
    "slot-b": {
      "opened": [],
      "active": null
    },
    "popout": {
      "opened": [{ "id": "ef56gh78", "appId": "file" }],
      "active": "ef56gh78"
    }
  }
}
```

注意三点：
1. **`popout` 是虚拟 slot**：`SlotContext.tsx:46` 的 `POPOUT_SLOT_ID = 'popout'`，复用同一套 slots shape（见 [03 章](./03-shell-and-chat.md) 抓手 5 和 [05 章](./05-slots-and-app-registry.md) 抓手 5）
2. **空 slot 也写**：`active: null` 表示空，不写 `opened` 字段（undefined 不进 jsonb）。但读时 `stored?.opened ?? seed` 兜底
3. **active 可以指向不存在于 opened 的 id**：`SlotContext.tsx:205-208` 的 `getState` 会兜底回退到 `opened[0]?.id`，不爆

### 抓手 4：instances 字段的三层 keying

`instance-state-store.ts:138-146`：

```typescript
type InstancesMap = Record<string, Record<string, unknown>>

function readPersistent(
  payload: { instances?: InstancesMap },
  instanceId: string,
  key: string,
): unknown {
  return payload.instances?.[instanceId]?.[key]
}
```

三层 keying：`profileId -> instances[instanceId][key]`。第一层是 profileId（workspace 或 fleet），第二层是 instanceId，第三层是具体 key（如 `cwd` / `viewingPath` / `drive`）。

存储示例（slot-a:files 实例存了 cwd 和 drive）：

```json
{
  "instances": {
    "ab12cd34": {
      "cwd": "/workspace/docs",
      "drive": "workspace",
      "sort": "modified-desc"
    },
    "slot-a:chat": {
      "draftMessage": "hello"
    }
  }
}
```

**为什么 instanceId 跨 workspace 会撞**：`SlotContext.tsx:199-202` 的种子 id `${slotId}:${appId}` 是确定性的，跨 workspace 完全相同。如果不加 profileId 维度，ws A 的 Settings 草稿会泄漏到 ws B 的 Settings。所以 profileId 必须是第一层 key（见 [05 章](./05-slots-and-app-registry.md) 抓手 7）。

**字段的值类型是 `unknown`**：因为不同 App 存不同东西。FilesApp 存 `cwd`（字符串）/`drive`（'workspace'|'sandbox'|...）/`sort`（枚举）；FileApp 存 `viewingPath`（字符串）。类型在各 App 内部用 `useInstancePersistentState<T>` 的泛型约束。

### 抓手 5：popoutGeo 字段

`SlotContext.tsx:31-36` 定义：

```typescript
interface PopoutGeo {
  x: number
  y: number
  w: number
  h: number
}
```

读：`SlotContext.tsx:186`：

```typescript
const popoutGeo = ((payload as { popoutGeo?: PopoutGeo }).popoutGeo ?? null) as PopoutGeo | null
```

写：`SlotContext.tsx:406-411`：

```typescript
const setPopoutGeoCb = useCallback(
  (geo: PopoutGeo) => {
    useWorkspaceProfileStore.getState().patch(workspaceId, { popoutGeo: geo })
  },
  [workspaceId],
)
```

消费：`components/shell/PopoutLayer.tsx:46-57`：

```typescript
x: popoutGeo?.x ?? -1,
y: popoutGeo?.y ?? -1,
w: popoutGeo?.w ?? DEFAULT_W,
h: popoutGeo?.h ?? DEFAULT_H,
// ...
useEffect(() => {
  if (!popoutGeo) return
  geo.current = { ...popoutGeo }
}, [popoutGeo])
```

`null`（未写）时用 `-1` 触发居中逻辑，`DEFAULT_W`/`DEFAULT_H` 是内置默认尺寸。popout 窗口拖动/缩放时 `setPopoutGeo` 写回 profile，下次打开复用位置。

### 抓手 6：wallpaper / chatSendKey 的 fleet 限定

`useWallpaper.ts:48-58`：

```typescript
export function useWallpaper(): { wallpaper: Wallpaper; setWallpaper: (w: Wallpaper) => void } {
  const payload = useWorkspaceProfile(FLEET_PROFILE_ID)  // ← 固定 fleet
  const stored = (payload as { wallpaper?: unknown }).wallpaper
  const wallpaper = isWallpaper(stored) ? stored : DEFAULT_WALLPAPER

  const setWallpaper = useCallback((w: Wallpaper) => {
    useWorkspaceProfileStore.getState().patch(FLEET_PROFILE_ID, { wallpaper: w })
  }, [])

  return { wallpaper, setWallpaper }
}
```

`useChatSendKey.ts:21-33` 同模式：

```typescript
export function useChatSendKey(): { mode: ChatSendKey; setMode: (m: ChatSendKey) => void } {
  const payload = useWorkspaceProfile(FLEET_PROFILE_ID)  // ← 固定 fleet
  const stored = (payload as { chatSendKey?: unknown }).chatSendKey
  const mode = isChatSendKey(stored) ? stored : DEFAULT_CHAT_SEND_KEY

  const setMode = useCallback((m: ChatSendKey) => {
    useWorkspaceProfileStore.getState().patch(FLEET_PROFILE_ID, { chatSendKey: m })
  }, [])

  return { mode, setMode }
}
```

**为什么这两个字段在 fleet**：注释（`useWallpaper.ts:44-47`、`useChatSendKey.ts:15-19`）原话：

> User-level desktop wallpaper preset, persisted on the fleet profile so the choice follows the user across workspaces and devices.

> User-level chat send-key preference, persisted on the fleet profile so the choice follows the user across workspaces and devices.

也就是"跟随用户"的偏好放 fleet（实际是 user_profile 表），"绑定 workspace"的状态放 ws（workspace_profile 表）。这是分层的判定原则：

| 字段类型 | 例子 | 放哪 |
|----------|------|------|
| 跟随用户的偏好 | wallpaper, chatSendKey, 主题色 | fleet（user_profile 表） |
| 绑定 workspace 的状态 | slots, instances, popoutGeo, layout_id | ws（workspace_profile 表） |

**例外**：`popoutGeo` 按理是跟随用户的（窗口位置偏好），但实际放在 ws。原因：fleet scope 和 ws scope 的 PopoutLayer 是分开挂的（见 [00 章](./00-overview.md) Provider 嵌套），fleet 的 popout 在 Desktop 内挂，ws 的 popout 在 WorkspacePage 内挂，几何分开存更直观。

### 抓手 7：patch 的 400ms 去抖 + 浅合并

`workspace-profile-store.ts:78-94`：

```typescript
patch(workspaceId, patch) {
  set((s) => ({
    byWorkspace: {
      ...s.byWorkspace,
      [workspaceId]: { ...(s.byWorkspace[workspaceId] ?? {}), ...patch },
    },
  }))

  pending.set(workspaceId, { ...(pending.get(workspaceId) ?? {}), ...patch })

  const existing = flushTimers.get(workspaceId)
  if (existing) clearTimeout(existing)
  flushTimers.set(
    workspaceId,
    setTimeout(() => void flush(workspaceId), FLUSH_DEBOUNCE_MS),
  )
}
```

`FLUSH_DEBOUNCE_MS = 400`（line 59）。三件事：

1. **立即更新本地 state**：`set(...)` 让订阅者马上看到新值。UI 不等 PATCH
2. **合并到 pending**：`pending.set` 把 patch 累积。如果 400ms 内有多次 patch，合并成一次 PATCH
3. **重置定时器**：已有 timer 就 clearTimeout，重新起 400ms 定时器。所以连续写入会一直推迟 PATCH，直到 400ms 静默

`flush` 函数（`workspace-profile-store.ts:97-111`）：

```typescript
async function flush(workspaceId: string) {
  flushTimers.delete(workspaceId)
  const patch = pending.get(workspaceId)
  if (!patch) return
  pending.delete(workspaceId)
  try {
    const merged = await backendFor(workspaceId).save(workspaceId, patch)
    useWorkspaceProfileStore.setState((s) => ({
      byWorkspace: { ...s.byWorkspace, [workspaceId]: merged },
    }))
  } catch (err) {
    console.error('[workspace-profile] flush failed', workspaceId, err)
    pending.set(workspaceId, { ...patch, ...(pending.get(workspaceId) ?? {}) })
  }
}
```

**失败重试**：PATCH 失败时把 patch 重新塞回 `pending`，但**不重设定时器**。下次有新 patch 才会触发再次 flush。这是"fail and retry on next write"策略，不是指数退避。

**后端浅合并**：`docs/wiki/control-plane/src/services/db/workspace-profile.md:26-43` 描述了 SQL：

```sql
INSERT INTO workspace_profile (workspace_id, payload, updated_at) VALUES ($1, $2::jsonb, now())
ON CONFLICT (workspace_id)
DO UPDATE SET payload = workspace_profile.payload || EXCLUDED.payload, updated_at = now()
RETURNING payload
```

PostgreSQL 的 jsonb `||` 操作符是**顶层浅合并**：只合并顶层 key，嵌套对象直接替换。所以：
- `patch({ slots: {...} })` 会**整体替换** `slots` 字段，不会内部合并
- `patch({ instances: { id1: {...} } })` 也会整体替换 `instances`，所以 `instance-state-store.ts:186-190` 必须先读 curInstances 再 spread 进 patch，不能只传增量

后端返回 merged payload，store 用 `setState` 替换本地。这是为了让服务器端的合并结果（包括其他 tab / 其他客户端的并发写入）反映回本地。

### 抓手 8：ensureLoaded 的单次守卫

`workspace-profile-store.ts:64-76`：

```typescript
ensureLoaded(workspaceId) {
  if (loadStarted.has(workspaceId)) return
  loadStarted.add(workspaceId)
  backendFor(workspaceId)
    .load(workspaceId)
    .then((payload) => {
      set((s) => ({ byWorkspace: { ...s.byWorkspace, [workspaceId]: payload } }))
    })
    .catch((err) => {
      loadStarted.delete(workspaceId) // allow retry
      console.error('[workspace-profile] load failed', workspaceId, err)
    })
}
```

**关键设计**：`loadStarted` Set 防重入。一次会话只拉一次 profile，重复调用直接 return。失败时才 `delete` 允许重试。

**配合 `useWorkspaceProfileLoaded`**（line 122-124）：

```typescript
export function useWorkspaceProfileLoaded(workspaceId: string | undefined): boolean {
  return useWorkspaceProfileStore((s) => (workspaceId ? workspaceId in s.byWorkspace : true))
}
```

`workspaceId in byWorkspace` 表示已加载完成（即使 payload 是空 `{}`）。Desktop.tsx 用这个 flag 决定是否挂 SlotProvider（见 [03 章](./03-shell-and-chat.md) 抓手 2）：

> Suppress slot/layout/dock rendering until the profile has been fetched at least once - otherwise we'd flash defaults before snapping to the user's saved choice.

## 端到端时序：一次 patch 从调用到落库

场景：用户在 Files app 里切目录，触发 `setCwd('/foo')`，到数据最终落到 PostgreSQL。

```
1. FilesApp 内部
   └─ const [cwd, setCwd] = useInstancePersistentState(instanceId, 'cwd', () => '/')
   └─ setCwd('/foo')
       └─ instance-state-store.ts:175-195 的 setValue
           ├─ curVal = readPersistent(payload, instanceId, 'cwd') = '/'  (旧值)
           ├─ resolved = '/foo'
           ├─ if (Object.is(curVal, resolved)) return  // 不是，继续
           ├─ nextInstances = { ...curInstances, [instanceId]: { ...curEntry, cwd: '/foo' } }
           └─ store.patch(workspaceId, { instances: nextInstances })
               ↓

2. workspace-profile-store.ts:78 patch
   ├─ set((s) => ({ ... byWorkspace[wsId] = { ...old, instances: nextInstances } }))  // 立即本地生效
   │   └─ useWorkspaceProfile(wsId) 订阅者重渲染
   │       └─ FilesApp 的 useInstancePersistentState 重新读，看到 cwd='/foo'
   │           └─ UI 立即切换目录（不等网络）
   ├─ pending.set(wsId, { ...(pending.get(wsId) ?? {}), instances: nextInstances })
   │   └─ 如果 400ms 内有别的 patch（比如 setSort），合并
   └─ clearTimeout(existing); flushTimers.set(wsId, setTimeout(flush, 400))
       ↓ (400ms 静默后)

3. workspace-profile-store.ts:97 flush
   ├─ flushTimers.delete(wsId)
   ├─ patch = pending.get(wsId); pending.delete(wsId)
   └─ backendFor(wsId).save(wsId, patch)
       │  ws scope: apiBackend.save
       │  fleet scope: fleetBackend.save -> api.patchUserProfile
       ↓

4. lib/api/client.ts PATCH /api/workspaces/:id/profile
   ├─ fetch(url, { method: PATCH, body: JSON.stringify(patch), credentials: include })
   └─ 后端 Hono 路由
       ↓

5. control-plane/src/routes/workspaces/profile.ts
   ├─ ACL 校验（canManage）
   ├─ patch = c.req.valid('json')  // WorkspaceProfilePayloadSchema，宽松
   └─ patchWorkspaceProfile(id, patch) -> SQL
       ↓

6. control-plane/src/services/db/workspace-profile.ts:17 patchWorkspaceProfile
   ├─ INSERT INTO workspace_profile (workspace_id, payload, ...) VALUES (...)
   ├─ ON CONFLICT (workspace_id) DO UPDATE
   │   SET payload = workspace_profile.payload || EXCLUDED.payload  -- jsonb 浅合并
   ├─ RETURNING payload
   └─ 返回 merged payload
       ↓

7. 前端收到 merged payload
   └─ workspace-profile-store.ts:104 setState
       └─ byWorkspace[wsId] = merged  // 服务器视角的最新值
           └─ 如果有其他 tab 并发写入，这里会反映出来
           └─ 订阅者再次重渲染（通常值没变，React 会 bailout）
```

**关键时间线**：
- 调用 `setCwd` 到 UI 看到新 cwd：**<1ms**（同步 zustand set）
- 调用 `setCwd` 到 PATCH 落库：**~400ms**（去抖等待）+ 网络往返
- PATCH 失败到下次重试：**等下次任意 patch 触发**（无主动重试）

## 怎么加一个新字段

假设要加 `theme: 'dark' | 'light'`，跟随用户的偏好。流程：

1. **决定层级**：跟随用户 → fleet；绑定 workspace → ws。假设是跟随用户。

2. **加 hook**（仿 `useWallpaper.ts`）：

```typescript
// hooks/useTheme.ts
import { FLEET_PROFILE_ID } from '@/stores/fleet-profile'
import { useWorkspaceProfile, useWorkspaceProfileStore } from '@/stores/workspace-profile-store'
import { useCallback } from 'react'

const THEMES = ['dark', 'light'] as const
type Theme = (typeof THEMES)[number]
const DEFAULT_THEME: Theme = 'dark'

function isTheme(v: unknown): v is Theme {
  return typeof v === 'string' && (THEMES as readonly string[]).includes(v)
}

export function useTheme(): { theme: Theme; setTheme: (t: Theme) => void } {
  const payload = useWorkspaceProfile(FLEET_PROFILE_ID)
  const stored = (payload as { theme?: unknown }).theme
  const theme = isTheme(stored) ? stored : DEFAULT_THEME

  const setTheme = useCallback((t: Theme) => {
    useWorkspaceProfileStore.getState().patch(FLEET_PROFILE_ID, { theme: t })
  }, [])

  return { theme, setTheme }
}
```

3. **不需要改后端**。`WorkspaceProfilePayloadSchema` 是宽松的，未知顶层 key 会被保留（jsonb 浅合并的 `||` 操作符保证）。

4. **不需要 migration**。首次读时 `stored` 是 undefined，fallback 到 DEFAULT_THEME，用户改了才写入。

5. **不需要改 `WorkspaceProfilePayload` 类型**。它是 `Record<string, unknown>`，断言 `as { theme?: unknown }` 就够。如果想加类型安全，可以在本仓库内定义一个 `WorkspaceProfileKnownFields` 接口，但不强制。

6. **改 `workspace-profile-store.ts:13-17` 的注释**。当前只列了 `layout_id` + `slots`，已经过时。把新字段加进去让下个人能看到清单。

## 延伸阅读建议

读完本章后：

- 想看后端 SQL 实现细节 -> 读 `docs/wiki/control-plane/src/services/db/workspace-profile.md`（43 行，最权威的数据库层文档）
- 想看 API 路由的契约 -> 读 `docs/wiki/control-plane/src/routes/workspaces/profile.md`（端点 + 设计意图）
- 想看 fleet profile 的 LS 迁移细节 -> 读 `stores/fleet-profile.ts:1-65`（注释极详细）
- 想看 slots 字段怎么被消费 -> 回 [03 章](./03-shell-and-chat.md) 抓手 3-8
- 想看 instances 字段怎么被消费 -> 回 [05 章](./05-slots-and-app-registry.md) 抓手 7
- 想看 layout FSM 的 real vs selected 怎么用 -> 读 `hooks/useLayoutState.ts`（158 行，注释完整）

**回到**：[INDEX](./INDEX.md) | [00 总体架构](./00-overview.md) | [03 Shell](./03-shell-and-chat.md) | [05 App Registry](./05-slots-and-app-registry.md)
