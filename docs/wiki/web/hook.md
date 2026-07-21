# hook.md - React Hook 决策框架

> 讲项目里反复出现的 React Hook 选择问题：什么时候用 useState、useMemo、useRef、Context；deps 怎么定；DOM 节点如何在组件树里共享。每条规则配项目里的真实代码做对照。读完应该能回答："我加一个新 hook 时该选哪个？deps 该放什么？DOM 要传给子组件时该用什么？"

---

## 一句话原则

**useState = 组件"拥有"的状态，useMemo = 从已有值"派生"的值，useRef = 不触发 re-render 的持有物，Context = 跨多层 / 不透明 children / 运行时挂载的共享通道**。

判断测试两个问题：

| 问题 | useState | useMemo |
|------|----------|---------|
| 这个值能从 props/state/其他 hook 结果"算"出来吗？ | 否（独立维护） | 是（派生） |
| 需要主动"改"它吗（响应用户操作）？ | 是（调 setState） | 否（依赖变自动重算） |

---

## 决策树

```
要存一个值 ──► 能从已有值算出来吗？
                │
       ┌────────┴────────┐
       │否                │是
       ▼                  ▼
   需要主动改吗？      有副作用/有状态吗？（订阅、连接、实例）
       │                  │
   ┌───┴───┐         ┌────┴────┐
   │是      │否      │是        │否
   ▼       ▼        ▼          ▼
 useState useRef    useState    useMemo
 (state)  (持有物)  (lazy init) (派生)
                                 │
                                 ▼
                       这个值要流到子组件参与渲染吗？
                                 │
                          ┌──────┴──────┐
                          │否            │是
                          ▼              ▼
                       自己用        后代是已知 shallow / 任意深度 / 运行时挂载？
                                       │
                                ┌──────┴──────┐
                                │shallow       │任意
                                ▼              ▼
                              props        Context + useXxx hook
```

**四个关键分支**：

1. **派生 + 无副作用** -> `useMemo`
   - 例：过滤后的列表、包装函数、组合对象
   - 重算无代价（行为等价），被 React 丢弃也无害

2. **派生 + 有副作用** -> `useState` + lazy initializer
   - 例：创建 zustand store / 建立订阅 / 创建 worker
   - 重算会丢状态/断订阅/创建第二个实例，必须严格一次

3. **不派生 + 独立维护** -> `useState`
   - 例：表单输入值、对话框 open 状态、loading 标志

4. **DOM 节点要流到子组件参与渲染** -> `useState` 当 ref callback + `Context`
   - 例：AppWindow 的 `headerSlot`（下文案例详讲）
   - `useRef` 的 mutation 不触发 re-render，下游拿不到新值

---

## 项目里的对照

| 场景 | 选择 | 为什么 |
|------|------|--------|
| `useWsApps` 算 AppDefinition[] | **useMemo** | 从 t/workspaceId/ws/catalog/... 派生，纯函数 |
| `useSessionNavigation` 包装 switchToSession | **useMemo** | 从 switchTo 派生包装函数，无副作用 |
| `AgentSessionContext.tsx:68` 创建 store | **useState** + lazy init | 派生但**有状态**（messages/SSE/订阅），重算会丢，必须严格一次 |
| `useInstanceState` 的 value/setter | **useSyncExternalStore** | 不派生，订阅外部 store |
| 对话框 open 状态 | **useState** | 用户操作改的，独立维护 |
| `WorkspaceChatPanel` 的 messages | **不缓存**（直接 useStore selector） | 不是组件拥有的，是订阅 zustand 切片 |
| `useWallpaper` 直接返回 `{ wallpaper, setWallpaper }` | **不缓存** | 字段简单，每次新对象无所谓 |
| `AppWindow` 的 `sectionRef`（测 fullscreenElement） | **useRef** | 只自己组件内部读，不需要触发 re-render |
| `AppWindow` 的 `headerSlot`（portal target） | **useState + Context** | DOM 要流到任意深度后代 + 插件，必须触发 re-render |

---

## 抓手 1：useState vs useMemo 的核心区分

### 不能用 useMemo 的场景：有副作用的实例

`AgentSessionContext.tsx:68-107`：

```typescript
const [store] = useState(() => {
  const deps: AgentSessionDeps = { ... }
  const s = createAgentSessionStore(workspaceId, deps, { chatEndpoint })
  if (initialSessionId) {
    s.setState({ activeSessionId: initialSessionId })
    useActiveSession.getState().switchTo(workspaceId, initialSessionId)
  }
  return s
})
```

为什么用 useState + lazy init 而不是 useMemo？

**React 文档明确说 useMemo 不保证只计算一次**。如果 React 丢弃缓存重算，会创建**第二个 store 实例**：

- 旧 store 里的 messages / isBusy / SSE 流 / 订阅者列表全没了
- 新 store 是空白的，要重新拉历史、重新连 SSE
- 旧 store 的 SSE 流变成孤儿（abort 不会被调）

这是**正确性问题**，不是性能问题。useState 的 lazy initializer **保证只执行一次**（在 mount 时），后续 re-render 直接返回缓存的 `[state]`，所以 store 实例引用永不变。

### 不能用 useRef 替代 useState 的场景（有状态实例）

技术上 useRef 也能缓存实例：

```typescript
const ref = useRef<StoreApi<...> | null>(null)
if (ref.current === null) {
  ref.current = createXxx()
}
```

但有两个劣势：

1. **闭包锁死**：ref.current 里的函数捕获首次渲染的 props。如果 props 变（如 workspaceId），ref 拿不到新值
2. **代码更啰嗦**：useState 一行解决，useRef 三行加 if 判断

`AgentSessionContext.tsx:61-66` 通过 `workspaceNameRef.current = workspaceName` 的模式让 deps 内部读 ref 拿最新 prop，同时 deps 对象身份保持稳定。这是 useRef 的正确用法："让闭包读到最新值"。

### 可以用 useMemo 的场景：纯派生

`useSessionNavigation` (`active-session-store.ts:57-67`)：

```typescript
export function useSessionNavigation() {
  const switchTo = useActiveSession((s) => s.switchTo)
  return useMemo(
    () => ({
      switchToSession: (workspaceId: string, sessionId: string) => switchTo(workspaceId, sessionId),
    }),
    [switchTo],
  )
}
```

这里返回的是 `{ switchToSession: ... }` 对象，每次都新对象字面量。useMemo 让 switchTo 不变时返回同一引用。即使被 React 丢弃重算，新对象的 switchToSession 内部调的还是同一个 switchTo（zustand action 引用稳定），行为等价。所以 useMemo 安全。

---

## 抓手 2：终极测试 - "被丢弃重算会怎样"

判断 useMemo vs useState 的终极标准：

> 如果 React 真的把 useMemo 缓存丢了重算一遍，会发生什么？

| 场景 | 重算结果 | 重算后果 | 用什么 |
|------|----------|----------|--------|
| 创建 zustand store | 新实例，有独立状态 | 丢 messages、SSE 断、订阅链乱 | **useState**（严格一次） |
| 包装纯函数（如 useSessionNavigation） | 新对象，闭包引用同一个 switchTo | 一次额外 re-render，行为等价 | **useMemo** 够用 |
| 计算派生数据（如 filteredList） | 新数组，内容相同 | 一次额外 re-render | **useMemo** 够用 |

判断准则：**重算的结果是否和旧结果"行为等价"**。

- 等价（无副作用、无内部状态）-> 用 useMemo
- 不等价（有状态、订阅、连接）-> 必须用 useState/useRef

---

## 抓手 3：useMemo deps 怎么定 - 3 步走

### Step 1：列出 memo 函数体里"读"了哪些变量

逐行扫函数体，找所有引用的外部变量。

### Step 2：对每个变量分类

| 变量来源 | 放 deps | 例子 |
|---------|---------|------|
| hook 参数 / props | ✅ | `workspaceId` |
| `useTranslation()` 的 `t` | ✅ | i18n 切语言时会变 |
| `useAuth()` 返回的 user | ✅（字段更精准） | `user?.role` |
| 其他 `useXxx` 的返回值 | ✅ | `catalog`, `config?.mcp_config` |
| `useSyncExternalStore` 的版本 | ✅ | `panelsVersion` |
| `useState` 的 setter | ❌ | React 保证稳定 |
| `useReducer` 的 dispatch | ❌ | React 保证稳定 |
| module-level import | ❌ | `getPanel`, `ChatApp`, 工具函数 |
| useRef 的 `.current` | ❌（用 ref 本身） | ref 引用稳定 |
| 局部计算的中间变量 | ❌ | `notRunning = ws?.status !== 'running'` |

### Step 3：决定放整个对象还是字段

```typescript
const { user } = useAuth()
// memo 里只用 user.role

// 选项 A（lint 推荐）：放整个对象
useMemo(() => { /* 读 user.role */ }, [user])

// 选项 B（精准）：放字段
useMemo(() => { /* 读 user.role */ }, [user?.role])
```

| 维度 | 选项 A（整个对象） | 选项 B（字段） |
|------|------------------|---------------|
| Lint 友好 | ✅ 自动通过 | ❌ 要 `biome-ignore` + 注释 |
| 触发频率 | 高（user 引用变就重算，即使 role 没变） | 低（role 变才重算） |
| 适用 | 字段多/嵌套深/不好列举 | 只用 1-2 个字段 |

**经验**：只用了 1-2 个字段就放字段（性能好），用了一大半字段就放整个对象（避免遗漏字段导致 bug）。

---

## 抓手 4：useWsApps 实战分析

`useWsApps.ts:70-256` 是最复杂的例子。逐项看 deps：

```typescript
return useMemo(() => {
  // 内部用到：
  // - workspaceId        ✅ 在 deps（hook 参数）
  // - t                  ✅ 在 deps（i18n）
  // - ws?.status         ✅ 在 deps（注意是字段，不是 ws 整个对象）
  // - config?.mcp_config ✅ 在 deps（字段，不是 config 整个对象）
  // - catalog            ✅ 在 deps
  // - user?.role         ✅ 在 deps（字段，不是 user 整个对象）
  // - panelsVersion      ✅ 在 deps
  // - lazyVersion        ✅ 在 deps
  // - getPanel           ❌ module-level import，稳定
  // - getLazyPanel       ❌ module-level import，稳定
  // - getPluginApp       ❌ module-level import，稳定
  // - ChatApp/FilesApp/… ❌ module-level import，稳定
  // - notRunning         ❌ 局部中间变量
}, [
  t, workspaceId, ws?.status, config?.mcp_config, catalog,
  user?.role, panelsVersion, lazyVersion,
])
```

### 为什么要 biome-ignore exhaustive-deps

```typescript
// biome-ignore lint/correctness/useExhaustiveDependencies: see above
return useMemo(() => { ... }, [t, workspaceId, ws?.status, ...])
```

exhaustive-deps lint 规则会要求放 `ws` / `config` / `user` **完整对象**。作者故意只放字段：

```typescript
// 方案 A（lint 推荐）：放完整对象
useMemo(() => { ... }, [ws, config, user, ...])
// 问题：workspaces 数组刷新 -> ws 是新对象（即使 status 没变）-> 触发重算

// 方案 B（实际写法）：只放字段
useMemo(() => { ... }, [ws?.status, config?.mcp_config, user?.role, ...])
// 好处：ws 引用变但 status 没变 -> Object.is(ws?.status, prev) 成立 -> 不重算
```

字段比对象"颗粒度更细"，能避免对象引用变化但实际内容没变时的无谓重算。代价是 lint 会报警（因为它无法静态证明你只用了这几个字段），所以要 `biome-ignore` + 注释说明。

### 一个反直觉点：panelsVersion 在函数体里没读但必须放

`useWsApps.ts:67-69`：

```typescript
// panelsVersion is intentionally a dep - it isn't read in the body but bumps
// when a plugin panel registers late, forcing the app list to recompute.
// biome-ignore lint/correctness/useExhaustiveDependencies: see above
```

`panelsVersion` 在 memo 函数体里**根本没读**（只读 `getPanel(pluginId)`），但必须放 deps。因为 panel 注册是异步的，bundle 加载完调 `registerPanel` 后 `panelsVersion` bump，这时 `getPanel(pluginId)` 才能返回真组件。如果不放，memo 不会重算，dock 里的插件 app 永远是 spinner。

这是"useMemo 当 reactivity 触发器用"的合法模式：**deps 不只是"满足闭包正确性"，也是"告诉 React 何时该重算"**。订阅外部 store 的版本号放进去就是这个目的。

---

## 抓手 5：常见反模式

### 反模式 1：派生值用 useState

```typescript
// ❌ 错误：派生值用 useState
const [filteredItems, setFilteredItems] = useState([])
useEffect(() => {
  setFilteredItems(items.filter(i => i.active))
}, [items])

// ✅ 正确：直接 useMemo
const filteredItems = useMemo(() => items.filter(i => i.active), [items])
```

派生值用 useState + useEffect 同步是 React 经典反模式：会多一次渲染（先渲染旧值，再渲染新值），且容易遗漏同步时机。

### 反模式 2：有状态的实例用 useMemo

```typescript
// ❌ 错误：store 实例用 useMemo
const store = useMemo(() => createXxxStore(), [])

// ✅ 正确：用 useState + lazy init
const [store] = useState(() => createXxxStore())
```

useMemo 可能被 React 丢弃重算，创建第二个 store 实例。store 是有状态的（messages、订阅），重算会丢。

### 反模式 3：闭包锁死

```typescript
// ❌ 错误：useRef 缓存函数，闭包锁死首次 props
const ref = useRef<(x: string) => void>()
if (!ref.current) {
  ref.current = (x) => doSomething(props.id, x)  // 锁住首次的 props.id
}

// ✅ 正确：useMemo 配 deps，props 变时重建
const fn = useMemo(
  () => (x: string) => doSomething(props.id, x),
  [props.id],
)
```

useRef 的闭包是首次渲染时的快照。如果依赖 props，必须用 useMemo + deps 让它在 props 变时重建。

### 反模式 4：deps 放完整对象但不必要

```typescript
// ❌ 不优：放整个 config 对象
useMemo(() => { /* 只用 config.mcp_config */ }, [config])
// config 引用每次 react-query 刷新都变 -> 无谓重算

// ✅ 更优：只放用到的字段
useMemo(() => { /* 只用 config.mcp_config */ }, [config?.mcp_config])
```

react-query 返回的对象引用频繁变（每次 invalidate 后 refetch 都新对象）。如果只用 1-2 个字段，放字段更精准。

### 反模式 5：deps 漏放

```typescript
// ❌ 错误：用了 userId 但没放 deps
const user = useAuth().user
const result = useMemo(() => loadUserData(workspaceId), [workspaceId])
// 内部如果读了 user.id（虽然这里没写），lint 会报但不一定 100% 捕获

// ✅ 正确：所有可能变化的外部值都放
const result = useMemo(() => loadUserData(workspaceId, user?.id), [workspaceId, user?.id])
```

漏放 deps 会导致 memo 用旧值重算，是隐蔽 bug。exhaustive-deps lint 规则就是防这个，除非有意识 biome-ignore + 注释，否则不要漏放。

### 反模式 6：DOM 节点要流到子组件却用 useRef

```typescript
// ❌ 错误：DOM 要传给 Context 却用 useRef
const headerSlotRef = useRef<HTMLDivElement | null>(null)
<div ref={(node) => { headerSlotRef.current = node }} />
<AppHeaderSlotContext.Provider value={headerSlotRef.current}>
  {/* ↑ mutation 不触发 re-render，下游永远拿到 null */}
</AppHeaderSlotContext.Provider>

// ✅ 正确：用 useState 当 ref callback
const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null)
<div ref={setHeaderSlot} />
<AppHeaderSlotContext.Provider value={headerSlot}>
  {/* ↑ div 挂载时 setHeaderSlot(<div>) 触发 re-render，下游拿到新值 */}
</AppHeaderSlotContext.Provider>
```

详见下文案例。

---

## 抓手 6：useEffect 的执行语义 - React 是单线程的

### 项目里的 compare-and-consume 模式

`web/src/stores/composer-store.ts:62-69`：

```typescript
useEffect(() => {
  if (!enabled || !request) return
  if (request.workspaceId !== workspaceId) return
  // Compare-and-consume: bail if it was already taken (another composer, or
  // StrictMode's second effect pass). Clearing it here is also what stops a
  // later remount of this composer from replaying the same insert.
  if (useComposerStore.getState().request !== request) return
  useComposerStore.setState({ request: null })
  // ...后续：把 request.text 插入 textarea 的 caret 位置...
}, [request, workspaceId, enabled, inputRef])
```

为什么 effect 里要先 `getState()` 比较再 `setState` 清空？看起来多此一举。要回答这个问题，先纠正一个常见误解。

### React 不是多进程，也不是多线程

整个 JS 运行时就是单线程 + 事件循环，"多个 composer 同时跑"在 React 里不会真并发。但有几个**看似并发、实则串行**的坑需要防御：

| 机制 | 是否真并发 | 是否需要防御 |
|---|---|---|
| 多进程 | ❌ 不存在 | 不需要 |
| 多线程 | ❌ 不存在 | 不需要 |
| StrictMode 双 effect | ❌ 串行，但跑两次 | ✅ 必须 |
| Concurrent 渲染 | ❌ 串行，可中断 | 不影响 effect |
| 多 composer remount 过渡 | ⚠️ 短暂共存 | ✅ 建议 |
| 闭包陈旧 | ❌ 串行 | ✅ 必须 |

### React 一次更新的两个阶段

```typescript
function Composer() {
  // ===== 阶段1：render phase（渲染阶段）=====
  // 这里是组件函数体。React 调用这个函数，算出 JSX
  const request = useComposerStore((s) => s.request)
  console.log('① 函数体跑了', request)   // ← 只读，不能搞副作用

  useEffect(() => {
    // ===== 阶段2：commit 之后 =====
    // DOM 已经更新完了，React 才跑这里
    console.log('② effect 跑了', request)  // ← 副作用放心搞
    useComposerStore.setState({ request: null })
  }, [request])

  return <div />
}
```

- **函数体**（阶段1）：React 只是要"算 JSX"，纯计算，**不允许有副作用**
- **effect**（阶段2）：DOM 都画完了，**专门给你搞副作用**的地方

### 一轮渲染的完整流程：A/B/C 兄弟节点例子

假设树结构是 `<Parent><A/><B/><C/></Parent>`，A 的 effect 里 `setState` 修改了一个影响 B 的值。完整流程：

```
====== 第 1 轮渲染 ======

阶段1：render phase（函数体，按树顺序）
  Parent 函数体跑 -> 算 JSX
  A 函数体跑 -> 算 A 的 JSX
  B 函数体跑 -> 算 B 的 JSX      ← B 在第 1 轮就渲染了！
  C 函数体跑 -> 算 C 的 JSX

阶段2：commit phase
  React 把 A/B/C 的输出挂到 DOM（一次性批量更新）

阶段3：effect phase（按树顺序跑，父先于子，深度优先）
  Parent 的 effect 跑
  A 的 effect 跑
    └─ A 调 setState（修改影响 B 的值）
       ↓ React 把这次更新塞进"调度队列"
       ↓ 但不会立刻中断当前 effect 链
  B 的 effect 跑          ← 还是跑！
  C 的 effect 跑          ← 还是跑！

====== 当前 effect 链跑完后，React flush 调度队列 ======

阶段4：第 2 轮 render（只渲染受影响的部分）
  B 函数体重跑            ← 因为 B 订阅的值变了
  A、C 不重跑（输入没变）

阶段5：commit B 的新 DOM

阶段6：B 的新 effect 跑（如果 deps 变了）
```

#### 关键细节：setState 不是同步生效的

A 的 effect 里调 `setState(x)`：

- ❌ 不会立刻让 B 重渲染
- ❌ 不会中断 B/C 的 effect
- ✅ 只是往 React 的"调度队列"塞一个更新
- ✅ 当前 effect pass 全部跑完后，React 才统一 flush 队列

这是 React 18 的 **automatic batching**：同一轮里多次 setState 会合并成一次重渲染。

#### effect 的执行顺序

effect 按树顺序跑（深度优先，父先于子）：

| 树结构 | effect 顺序 |
|---|---|
| `<Parent><A/><B/><C/></Parent>` | Parent -> A -> B -> C |
| `<A><B><C/></B></A>` | A -> B -> C |
| `<A><B/></A>` 然后 `<C/>` 兄弟 | A -> B -> C |

清理函数（cleanup）反序：子先于父，等同组件先 unmount 再 mount 时的反向遍历。

#### 这个模式往往是反模式

"A 的 effect 里 setState -> 触发 B 重渲染" 这种链路，在 React 文档里被点名是 [You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect) 的典型反模式。

常见替代：

| 反模式 | 替代方案 |
|---|---|
| effect 里 setState 同步派生值 | `useMemo` 直接派生 |
| effect 里 setState 修正 props | 直接在 render 里算 |
| effect 里通知父组件 | 把 state 提升到父组件，父直接传 props |
| effect 里订阅外部 store | `useSyncExternalStore` |

但 `composer-store.ts` 的 `setState({ request: null })` **不是反模式**：它是**消费一次性事件**（外部面板请求插入文本），不是派生计算。这才是 effect 该干的活。

> 一句话闭环：一轮 render = 整棵树（受影响部分）的函数体跑一遍 -> commit -> 所有 effect 按树顺序跑一遍。effect 里的 setState 不会立刻触发重渲染，而是塞进调度队列，等当前 effect 链全部跑完后 React 才统一 flush，进入下一轮 render。

### Concurrent 渲染只影响 render phase

Concurrent 模式下，React 可以**打断阶段1**：

```
① 函数体跑了  ← 跑到一半被打断
                ↓ 被高优先级任务插队
① 函数体跑了  ← React 重新跑一遍函数体
② effect 跑了  ← commit 之后只跑这一次
```

函数体跑了两遍，effect 只跑了一遍。因为函数体被打断后 React 没法继续算下去，只能丢掉重算；但 effect 是 commit 成功之后才跑的，commit 成功了就不会重跑 effect。

> 顶层逻辑：函数体是"草稿"，草稿可以撕了重写；effect 是"定稿盖章"，盖了就不重盖。

**对副作用的影响**：

| 副作用写在哪里 | Concurrent 模式后果 |
|---|---|
| 函数体里（如 `fetch('/api/log')`） | ❌ 跑两次，是 bug |
| effect 里（如 `setState({request:null})`） | ✅ 只跑一次，安全 |

所以 `composer-store.ts:69` 的 `setState({ request: null })` 写在 effect 里，Concurrent 渲染影响不到它。真正能威胁到它的是 **StrictMode 双跑 effect**。

### StrictMode 双调用 effect（最常见的坑）

只在 **dev 模式**下，React 18+ 会有意地：

```
mount -> effect 跑一遍 -> unmount(假) -> cleanup -> mount(假) -> effect 再跑一遍
```

这是**同一个 composer 实例**，不是两个 composer。但因为 effect 跑了两次，如果不消费 `request`，就会插入两次同一段文本。这是 compare-and-consume 要防御的主要场景。

### 多 composer 实例（极少但可能）

只有两种情况会出现"两个 composer 同时活着"：

**a. 你真的在树里挂了两次** `<Composer workspaceId="x" />`，开发者写错了。

**b. remount 过渡期**：slot 切换时，旧的 composer 还没完全 unmount、新的已经 mount，短暂共存。这也是 `composer-store.ts` 顶部注释说的场景（`SlotContainer` 切换 active app）。

两个 composer 都订阅了同一个 store，effect 跑的顺序是确定的（按树顺序），但谁先抢到 `request` 谁就消费，另一个 `getState().request !== request` 自动放弃。

### 闭包 vs 实时值

```typescript
const request = useComposerStore((s) => s.request)  // 渲染时的快照

useEffect(() => {
  // 闭包捕获的 request 是渲染那一刻的值
  // 但在 effect 执行的那一刻，store 可能已经被别人改了
  if (useComposerStore.getState().request !== request) return  // ← 必须读实时值
  useComposerStore.setState({ request: null })
}, [request])
```

订阅拿到的 `request` 是**渲染快照**；`getState()` 是**当前实时值**。两者不等，说明"令牌已经被别人拿走了"。这是为什么比较必须用 `getState()` 而不是闭包里的 `request`。

### 闭环：compare-and-consume 防的到底是什么

| 防御对象 | 怎么防 |
|---|---|
| StrictMode 双 effect | 第二次跑时 `getState().request` 已经是 null，bail |
| 多 composer 抢同一个 request | 后跑的 `getState().request !== request`，bail |
| remount 重放 | 第一次消费后清空，remount 时 store 已空 |
| 闭包陈旧 | 用 `getState()` 读实时值，不用闭包里的快照 |

> 一句话闭环：React 里你担心的不是并发竞争，是 **effect 被重复触发 + 闭包陈旧** 这两个串行问题。compare-and-consume 是最便宜的解法：用"令牌一次性"语义同时兜住这两个坑。

---

## 案例：AppWindow header slot 模式

上面 5 个抓手讲的是"在单个组件内"的 hook 决策。下面这个案例讲"DOM 节点如何跨组件共享"的 hook 决策。

### 背景：AppWindow 的 header slot 模式

`web/src/components/shell/windows/AppWindow.tsx:60-126` 关键代码：

```tsx
export function AppWindow({ children, onClose, ... }: AppWindowProps) {
  const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null)
  //         ↑ state 存 DOM 节点
  const sectionRef = useRef<HTMLElement>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)

  return (
    <section ref={sectionRef}>
      <header>
        {/* 这一行：把 setState 函数当 ref callback 用 */}
        <div ref={setHeaderSlot} className="flex min-w-0 flex-1 items-center gap-1.5" />
        <WindowControls onClose={onClose} ... />
      </header>
      {/* 把 DOM 节点通过 Context 流给任意深度的后代 */}
      <AppHeaderSlotContext.Provider value={headerSlot}>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
      </AppHeaderSlotContext.Provider>
    </section>
  )
}

export const AppHeaderSlotContext = createContext<HTMLDivElement | null>(null)
export function useAppHeaderSlot(): HTMLDivElement | null {
  return useContext(AppHeaderSlotContext)
}
```

**这条链路的设计目的**：

- AppWindow 自己渲染一个空的 header `<div>`（占位）
- 业务后代组件通过 `useAppHeaderSlot()` 拿到这个 div
- 用 `createPortal(<AppHeaderButton .../>, headerSlot)` 把按钮"塞"进 AppWindow 的 header 里
- 解耦了"chrome（壳）的拥有者"和"内容（业务按钮）的拥有者"

### Q1：为什么 ref 用 useState 不用 useRef

**因为 `headerSlot` 不只是自己内部用，它要作为值通过 Context 流到子组件。`useRef` 的 mutation 不触发重渲染，下游 Context 消费者就拿不到这个 DOM 节点。**

#### useRef 会怎么坏

```tsx
// ❌ 假设改成 useRef
const headerSlotRef = useRef<HTMLDivElement | null>(null)

<div ref={(node) => { headerSlotRef.current = node }} ... />
<AppHeaderSlotContext.Provider value={headerSlotRef.current}>
  <div>{children}</div>
</AppHeaderSlotContext.Provider>
```

执行流程：

```
T0  首次 render：
       headerSlotRef.current = null
       <Provider value={null}>     ← 把 null 作为 value 传下去
       子组件 useContext 拿到 null
       createPortal(<按钮/>, null)  ❌ portal 目标是 null，按钮不渲染

T1  React 挂载 <div>，调用 ref callback：
       headerSlotRef.current = <div>    ← mutation！
       ❌ React 不知道这事发生了，没有 schedule re-render
       ❌ Provider 的 value 还是上次 render 时的 null
       ❌ 子组件拿到的还是 null
       ❌ 永远 portal 不进去
```

**根本原因**：React Context 用 `Object.is` 比较 `value` 是否变化来决定要不要让消费者重渲染。`useRef.current = node` 是 mutation，**Provider 的 `value` 引用没变**（一直是 `headerSlotRef.current`，引用层面永远是那个 ref 对象），React 不会重渲染下游。

#### useState 怎么解决

```tsx
const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null)
<div ref={setHeaderSlot} ... />    // setHeaderSlot 当 ref callback
```

执行流程：

```
T0  首次 render：
       headerSlot = null（state 初值）
       <Provider value={null}>
       子组件 useContext 拿到 null
       createPortal(<按钮/>, null)  -> 按钮暂时不渲染

T1  React 挂载 <div>，调用 ref callback：
       ref callback 就是 setHeaderSlot 函数
       setHeaderSlot(<div DOM 节点>)   ← 调用 setState！
       ✅ React 知道 state 变了，schedule re-render

T2  第二次 render：
       headerSlot = <div DOM 节点>     ← 新的 state 值
       <Provider value={<div DOM>}>    ← value 引用变了
       ✅ Context 消费者重渲染
       子组件 useContext 拿到 <div DOM>
       createPortal(<按钮/>, <div DOM>)  ✅ 按钮渲染进去了
```

#### useState-as-ref 这个 idiom 的细节

`setHeaderSlot` 是 useState 返回的 setter，签名 `(newValue) => void`。React 把它当 ref callback 用时：

```
挂载时：setHeaderSlot(<div>)     ← state 变成 DOM 节点
卸载时：setHeaderSlot(null)       ← state 变成 null
```

跟标准 ref callback `(node: T) => void` 兼容（React 只用第一个参数）。卸载场景也对：state 回到 null，下游 `createPortal(<按钮/>, null)` 也跟着不渲染。

> 这就是为什么 React 官方文档推荐"如果 ref 的值需要参与渲染，就用 useState 当 ref callback"。详见 [You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)。

### Q2：为什么用 Context 不用 props

**因为 `headerSlot` 要从 `AppWindow` 流到任意深度的后代组件（包括运行时挂载的插件），中间层 `children` 是外部传进来的不透明黑盒，没法 prop drill。Context 让"传递路径"和"使用与否"完全解耦。**

#### 先澄清一个误解

到 `<AppHeaderSlotContext.Provider value={headerSlot}>` 这一行时，`headerSlot` **已经不是 ref 了**。它是 state 里存的 DOM 节点本身。ref 只是上面 `ref={setHeaderSlot}` 那一步把 DOM 节点抓进 state 的工具。所以这里讨论的是"为什么 DOM 节点用 Context 传而不是 props 传"，跟 ref 无关。

#### 用 props 的话会发生什么

**方案 A：render prop**

```tsx
export function AppWindow({ children: render, ... }: AppWindowProps) {
  const [headerSlot, setHeaderSlot] = useState(null)
  return (
    <section>
      <header><div ref={setHeaderSlot} /></header>
      {render(headerSlot)}     // ← 把 headerSlot 通过 render prop 传出去
    </section>
  )
}

// 调用方
<AppWindow>
  {(headerSlot) => <AdminPanel headerSlot={headerSlot} />}
</AppWindow>
```

问题：

- 破坏了 `<AppWindow><AdminPanel/></AppWindow>` 这种自然的 children 写法，所有调用方都得改成 render prop
- `AdminPanel` 还要把 `headerSlot` 一路 prop drill 给内部真正用它的地方

**方案 B：纯 prop drilling**

```tsx
function AdminPanel({ headerSlot }: { headerSlot: HTMLDivElement | null }) {
  return <Tabs><TabsList><ConnectorsSection headerSlot={headerSlot} /></TabsList></Tabs>
}
function ConnectorsSection({ headerSlot, ... }: ...) {
  return <ResourceFilterTabs headerSlot={headerSlot} />
}
function ResourceFilterTabs({ headerSlot, ... }: ...) {
  // 真正要用 createPortal(<AppHeaderButton/>, headerSlot) 的地方
}
```

问题：

- **每一层都得加 `headerSlot` prop**，即使中间层根本不用它，只是搬运工
- 项目里有 7+ 个 App section（AdminPanel / ConnectorsSection / MemoryStoresPanel / SkillsSection / SkillDetailView / AfsSharesPanel / WorkspaceChatPanel ...）每个都要改
- 类型签名变长，每个组件都得声明这个 prop
- 新增使用方要改一连串文件

#### Context 怎么解决

```tsx
// 提供方：AppWindow
<AppHeaderSlotContext.Provider value={headerSlot}>
  <div>{children}</div>
</AppHeaderSlotContext.Provider>

// 消费方：任何深度任意位置
function AnyDeepComponent() {
  const headerSlot = useAppHeaderSlot()   // ← 一行拿到
  return createPortal(<AppHeaderButton/>, headerSlot)
}
```

特点：

| 维度 | Props | Context |
|---|---|---|
| 中间层组件需要改动 | ✅ 每层加 prop | ❌ 不需要 |
| 不使用的组件知道这事 | ✅ 知道（被迫声明 prop） | ❌ 不知道 |
| 多个消费方独立取用 | 每个独立 prop drill | 各自调 hook |
| 运行时挂载的插件拿得到 | ❌ 没法 | ✅ 直接调 |

#### 关键场景：插件系统

看 `web/src/lib/host.ts:100`：

```tsx
windows: { useAppHeaderSlot, AppHeaderButton }
// 挂到 window.tos 上
window.tos = host
```

这把 `useAppHeaderSlot` 暴露给**运行时加载的插件**。插件代码根本不认识 AppWindow，也没法 prop drill。它只能这么写：

```tsx
// 某个运行时加载的插件
const headerSlot = window.tos.windows.useAppHeaderSlot()
createPortal(<插件按钮/>, headerSlot)
```

只有 Context 能支持这种"在 React 树上挂一个全局可访问的洞，任意后代（包括运行时挂载的）都能 `useContext` 拿到"。

#### 关于性能

Context 的 `value` 变化会让所有消费者重渲染。这里完全可接受：

- `headerSlot` 只在 div 挂载时从 `null` 变 `<div>`，**一次性事件**，不是高频
- AppWindow 通常 1-3 个，子组件数十个，不是性能热点
- 这恰恰是需要的：div 挂载后下游必须重渲染才能 portal 进去

### 闭环：从 div 挂载到按钮 portal 进去

```
T0  AppWindow 首次 render
       headerSlot state = null
       <div ref={setHeaderSlot}> 还没挂载
       Provider value={null}
       下游 useAppHeaderSlot() 返回 null
       下游 createPortal(<按钮/>, null) -> 按钮暂时不渲染

T1  React 把 <div ref={setHeaderSlot}> 挂载到 DOM
       React 调用 ref callback = setHeaderSlot(<div>)
       setHeaderSlot 内部：state = <div>
       ✅ React schedule re-render（useState 的特性）

T2  AppWindow 第二次 render
       headerSlot = <div>
       Provider value={<div>}     ← value 引用变了
       ✅ Context 消费者重渲染
       下游 useAppHeaderSlot() 返回 <div>
       下游 createPortal(<按钮/>, <div>)
       ✅ 按钮渲染进 AppWindow 的 header 里
```

---

## 通用规则：什么时候用哪个

| 场景 | 用什么 | 例子 |
|---|---|---|
| 自己组件内部读 DOM，做测量 / focus / 滚动 | `useRef` | `sectionRef`（AppWindow.tsx:70）测 fullscreenElement |
| DOM 节点要流到子组件参与渲染（Context value / props / derive） | `useState` 当 ref callback | `headerSlot`（AppWindow.tsx:69） |
| 跨多层 / 不透明 children / 运行时挂载 | `Context` + `useXxx` hook | `AppHeaderSlotContext`（AppWindow.tsx:48-52） |
| 父子直接传递、层数浅、已知消费方 | `props` | `<WindowControls onClose={onClose}/>` |
| 跨层级但只有 1-2 个固定消费方 | `props` 或 `Context` 都行，看可读性 | - |
| 派生纯函数 / 派生数组 | `useMemo` | `useSessionNavigation` |
| 有状态实例（store / 订阅） | `useState` + lazy init | `AgentSessionContext.tsx:68` |
| 独立状态（表单值 / open 标志） | `useState` | 对话框 open |

判断顺序：

1. **要不要让 React 知道这个值变了**？不要 -> `useRef`；要 -> `useState`
2. **谁要用**？只自己用 -> state/ref 都行；后代要用 -> Context 或 props
3. **后代是已知 shallow 还是 opaque/任意深度**？shallow + 固定 -> props；opaque / 深层 / 运行时 -> Context
4. **是派生还是有状态实例**？派生无副作用 -> useMemo；有状态 -> useState lazy init

---

## 端到端决策流程

假设你要写一个新 hook，决策顺序：

```
Step 1：这个值是派生还是独立维护？
  ├─ 独立维护（用户输入、网络结果）-> useState
  └─ 派生（从已有值算）-> Step 2

Step 2：派生过程中有没有副作用/有状态实例？
  ├─ 有（创建 store、订阅、连接）-> useState + lazy init
  └─ 无（纯计算、包装函数）-> useMemo

Step 3：useMemo 的 deps 怎么定？
  ├─ Step 3a：列函数体里读的变量
  ├─ Step 3b：分类（外部值放 / 稳定引用不放 / 中间变量不放）
  └─ Step 3c：决定整个对象 vs 字段（1-2 字段就放字段，多字段就放对象）

Step 4：终极测试
  └─ 如果 React 把这个 useMemo 缓存丢了重算，行为等价吗？
      ├─ 等价 -> useMemo 安全
      └─ 不等价（丢状态/断订阅）-> 改用 useState

Step 5：这个值要流到子组件吗？
  ├─ 不要 -> 自己用 useRef/useState 都行
  └─ 要 -> 后代是已知 shallow？
      ├─ shallow + 固定消费方 -> props
      └─ opaque / 任意深度 / 运行时挂载 -> Context + useXxx hook

Step 6：DOM 节点要参与渲染吗？
  ├─ 只自己内部用 -> useRef
  └─ 要流到 Context/props -> useState 当 ref callback（不是 useRef）
```

---

## 最终决策口诀

```
1. 能派生 -> useMemo；要持有 -> useState
2. 有副作用/有状态 -> 即使是派生也用 useState（lazy init）
3. deps 放"会变的外部值"，不放"稳定的 import"和"中间变量"
4. 只用几个字段 -> 放字段 + biome-ignore；用一大半字段 -> 放整个对象
5. 订阅外部 store -> 把版本号放进 deps 驱动重算（即使函数体没读它）
6. DOM 要流到子组件 -> useState 当 ref callback，不用 useRef
7. 跨多层/不透明 children/运行时插件 -> Context，不 prop drill
```

**两个终极测试**：

- **useMemo 安全性测试**：如果 React 真的把 useMemo 缓存丢了重算，行为等价吗？等价 -> 安全；不等价 -> 必须 useState
- **useState-as-ref 必要性测试**：这个 DOM 节点只是自己读，还是要流到 Context/props 参与渲染？只自己读 -> useRef；要流 -> useState

这 7 条口诀 + 2 个终极测试，覆盖 95% 的场景。剩下 5% 是边缘情况（concurrent mode、Suspense 边界、ref callback cleanup 等），按需查文档。

---

## 延伸阅读

读完本章后：

- 想看 useState + lazy init 创建 store 的实例 -> 读 `stores/AgentSessionContext.tsx:68-107`
- 想看 useMemo 派生数组 + biome-ignore 的实例 -> 读 `hooks/useWsApps.ts:70-256`
- 想看 useMemo 包装函数返回稳定引用 -> 读 `stores/active-session-store.ts:57-67`
- 想看 ref 拉通最新 prop 的模式 -> 读 `stores/AgentSessionContext.tsx:61-66`（workspaceNameRef / syncRef / onSessionCreatedRef）
- 想看 useSyncExternalStore 桥接非 React 注册表 -> 读 `hooks/useWsApps.ts:61-65` + `lib/panel-registry.ts`
- 想看 useState-as-ref + Context 共享 DOM 的实例 -> 读 `components/shell/windows/AppWindow.tsx:60-126`
- 想看 Context 暴露给运行时插件的实例 -> 读 `lib/host.ts:100` + `window.tos`
- 想看 useEffect compare-and-consume 模式（防 StrictMode 双跑、多实例竞争、闭包陈旧）-> 读 `stores/composer-store.ts:44-100`
- 想看官方对 useMemo 不保证稳定性的说明 -> 读 React 文档 [You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect) 与 [useMemo](https://react.dev/reference/react/useMemo) 章节
