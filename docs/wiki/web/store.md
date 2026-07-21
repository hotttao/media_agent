# store.md - Web 项目里的 Store 用法

> 讲 `web/src/stores/` 里 9 个 store 文件归纳出的 6 种用法。读完应该能回答："我要存一个跨组件的状态，该选哪种写法？什么时候用 persist、什么时候用工厂、什么时候用 Context？"

---

## 前置：什么是 store，为什么不用 useState

### useState 的局限

React 自带的 `useState` 是**组件内**状态 -- 组件卸载就没了，跨组件用要层层 props 传递。共享状态一多，组件树就变成"props 中转站"。

### store 是什么

**store = 组件外的可变状态盒**。放在模块作用域或 Context 里，三个特点：

- 跨组件读写（不用 props 传递）
- 跨生命周期存活（组件卸载状态还在）
- 跨渲染边界（多处订阅同一份数据，永远一致）

### store 里面有什么

一个 store 本质是个对象，包含**两部分**：

| 部分 | 作用 | 例子 |
|---|---|---|
| **state** | 数据本身 | `count: 0`、`messages: []` |
| **actions** | 修改 state 的方法 | `inc: () => set(...)`、`sendMessage: ...` |

调用 `create((set) => ({ count: 0, inc: ... }))` 时，state 和 actions 写在同一个返回值里，类型上拼成一个 `State & Actions` 接口。

### StoreApi：store 对外的 4 个方法

`create()` / `createStore()` 返回的不只是一个 hook，还是一个 **StoreApi 对象**，对外暴露 4 个方法（脱离 React 也能用）：

| 方法 | 作用 |
|---|---|
| `getState()` | 读当前 state 快照 |
| `setState(partial \| (prev) => partial)` | 改 state，触发订阅者 |
| `subscribe(listener)` | 订阅 state 变化，返回取消订阅函数 |
| `getInitialState()` | 拿初始 state 快照 |

### React 怎么订阅 store

React 里用的 `useXxxStore(selector)` hook 就是**对上面 4 个方法的封装**：

```
useXxxStore(selector)  ≈  useSyncExternalStore(
                            store.subscribe,
                            () => selector(store.getState())
                          )
```

意思：用 React 的 `useSyncExternalStore` 注册订阅，store 一变就从 `getState()` 取切片，传给组件触发重渲染。

**关键洞察**：store 本身不依赖 React -- `getState / setState / subscribe` 在事件回调、定时器、Promise 里都能直接用。React 只是 store 的一个订阅者。

### 本项目用的库

本项目所有 store 用 **Zustand** 实现。最小代码：

```ts
import { create } from 'zustand'

const useX = create<State & Actions>((set) => ({
  // state
  count: 0,
  // actions
  inc: () => set((s) => ({ count: s.count + 1 })),
}))

// 组件里
const count = useX((s) => s.count)   // 订阅切片
const inc = useX((s) => s.inc)       // 拿 action
```

### 看真实项目里的 `agent-session-store.ts` 怎么创建 store

项目里最复杂的 store 是 `web/src/stores/agent-session-store.ts:187` 的 `createAgentSessionStore`，三个关键模式必须看懂：

#### 模式 1：双括号 `createStore<T>()(() => ({ ... }))`

```ts
const store = createStore<AgentSessionSlice>()(() => ({
  workspaceId,            // state
  activeSessionId: undefined,
  // ...
  switchSession: async (id, ctx) => { /* action */ },  // action
}))
```

为什么两层 `()`？

| 位置 | 含义 |
|---|---|
| 第一层 `()` | TypeScript 类型参数占位（curry 技巧，让 TS 能从 initializer 反推类型） |
| 第二层 `(() => ({...}))` | 真正的 initializer 函数，返回 state + actions 对象 |

跟普通 `create` 的区别：`create` 直接返回 hook，`createStore` 返回 **StoreApi 对象**（前面表里的 4 个方法）。这是工厂模式能用、能在 React 外调的前提。

#### 模式 2：闭包变量 + 自引用

工厂函数内部除了 store 本身，还能定义**辅助函数和闭包变量**，actions 可以引用它们：

```ts
export function createAgentSessionStore(workspaceId, deps) {
  // 闭包变量 -- 跟 store 实例同生命周期
  let abortController: AbortController | null = null
  let switchVersion = 0

  // 辅助函数 -- 在多个 action 里复用
  function abortActiveStream() {
    abortController?.abort()
    switchVersion++
  }

  // store 自己 -- actions 通过 store.getState() 读最新 state
  const store = createStore<Slice>()(() => ({
    activeSessionId: undefined,
    // ...
    async switchSession(id) {
      abortActiveStream()                    // ← 调闭包辅助函数
      const cur = store.getState().activeSessionId   // ← store 自引用
      if (id === cur) return
      store.setState({ activeSessionId: id })         // ← store 自引用
    },
  }))

  return store
}
```

**两个为什么**：

1. **为什么 action 里要 `store.getState()` 而不直接读 state** -- action 是定义在 initializer 里的函数，**被调用时** state 可能已经变了（异步、用户连续操作），必须实时读快照。直接闭包读 `activeSessionId` 会拿到旧值。
2. **为什么辅助函数能引用 store** -- JS 闭包特性：`store` 是 `const`，先声明再被引用，等 action 真正执行时 `store` 已经赋值完毕。

#### 模式 3：依赖注入 `deps`

```ts
export function createAgentSessionStore(
  workspaceId: string,
  deps: AgentSessionDeps,     // ← API / SSE / effects 全从外面传进来
  options?: { ... },
): StoreApi<AgentSessionSlice>
```

`deps` 把所有外部依赖（HTTP API、SSE 工厂、副作用回调）作为参数传进来，store 不直接 import 它们。**好处**：单测时传 mock deps 就能测，不需要真实后端（看 `agent-session-store.test.ts`）。

#### 一图总结创建流程

```
createAgentSessionStore(workspaceId, deps)
  │
  ├── 1. 定义闭包变量        abortController / switchVersion / ...
  ├── 2. 定义辅助函数        abortActiveStream / buildSSEHandlers / ...
  ├── 3. createStore<T>()(() => ({
  │        state:    workspaceId, activeSessionId, messages, ...
  │        actions: switchSession, sendMessage, ...  ← 闭包引用 store.getState()
  │      }))
  └── 4. return store   (StoreApi 对象)
```

调用方（`AgentSessionContext.tsx:99`）拿到 `store` 后塞进 React Context，子组件用 `useStore(store, selector)` 订阅 -- 这就是第 4 种用法（Context + vanilla store）。

### `create` vs `createStore`：什么时候用哪个

到目前为止出现了两种创建 store 的 API，对比清楚：

| 维度 | `create`（`zustand`） | `createStore`（`zustand/vanilla`） |
|---|---|---|
| 返回值 | React hook `useXxx(selector)` | StoreApi 对象 |
| 实例数量 | 全局一个（模块作用域单例） | 每次调用新实例 |
| React 外能用吗 | ❌ hook 只能在组件里调 | ✅ 任何地方都能 `getState/setState` |
| 要不要配 Context | 不需要 | 必须配 Context 才能让子组件订阅 |
| 典型场景 | 全局偏好、跨组件共享一个值 | 工厂模式（每 workspace 一份） |

**选型口诀**：全局只有一个用 `create`，每个实例独立用 `createStore + Context`。

### store 跟 React 的关系

很多人以为 store 是 React 的一部分 -- 不是。它们是**两个独立的层**：

```
┌──────────────────────────────────────────────────┐
│  你的业务代码                                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐         │
│  │ 事件回调  │  │ 定时器   │  │  Promise │         │
│  │setState()│  │getState()│  │  setState│         │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘         │
│       └─────────────┼─────────────┘                │
│                     │                              │
│                     ▼                              │
│           ┌──────────────────┐                     │
│           │      store       │  ← Zustand          │
│           │   (vanilla)      │    不依赖 React      │
│           │  getState /       │                     │
│           │  setState /       │                     │
│           │  subscribe        │                     │
│           └────────┬─────────┘                     │
│                    │                               │
│       ┌────────────┴────────────┐                  │
│       ▼                         ▼                  │
│ ┌──────────────┐         ┌──────────────┐         │
│ │  React 组件  │         │   Node.js    │         │
│ │ useXxxStore()│         │  直接调 API  │         │
│ │  (订阅者)    │         │              │         │
│ └──────────────┘         └──────────────┘         │
└──────────────────────────────────────────────────┘
```

**三个关键认知**：

1. **store 是外部状态源** -- React 只是它的一个订阅者，不是它的宿主
2. **`useSyncExternalStore`** 是 React 18 官方提供的"订阅外部 store"的钩子，Zustand 的 `useXxxStore` 就是基于它实现的
3. **脱离 React 也能用** -- 在 Node.js、单测、worker 里直接调 `store.getState/setState`，不需要 React

### 内部实现：极简版 Zustand

Zustand 的核心代码其实很少。简化掉边界处理后长这样：

```ts
// ── zustand/vanilla 核心 ──
function createStore<T>(initializer) {
  let state: T
  const listeners = new Set<() => void>()

  // ↓ 这就是 create((set) => ...) 里那个 set 参数
  const setState = (next) => {
    const partial = typeof next === 'function' ? next(state) : next
    state = { ...state, ...partial }   // 浅合并
    listeners.forEach((l) => l())      // 通知所有订阅者
  }
  const getState = () => state
  const subscribe = (listener) => {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }

  const api = { getState, setState, subscribe, getInitialState: () => state }

  // initializer 接收 (set, get, api) 三个参数
  // 用户写 create((set) => ({...})) 时，set 就是上面的 setState
  state = initializer(setState, getState, api)
  return api
}

// ── zustand 的 useStore hook 核心 ──
function useStore<T, U>(store, selector) {
  return useSyncExternalStore(
    store.subscribe,                       // React 用这个订阅
    () => selector(store.getState()),      // 读切片
  )
}

// ── create = createStore + 包成 hook（闭包绑死实例）──
function create<T>(initializer) {
  const store = createStore(initializer)           // 内部还是 createStore
  function useBoundStore<U>(selector: (s: T) => U): U {
    return useStore(store, selector)              // 闭包绑死这个 store
  }
  return useBoundStore
}
```

**`create((set) => ({ ... }))` 里的 `set` 是什么？**

`set` 就是 `createStore` 内部定义的 **`setState` 函数**，通过 `initializer(setState, getState, api)` 的第一个参数传给你的 initializer。它接受两种形式：

```ts
// 形式 1：直接给对象（浅合并到 state）
inc: () => set({ count: 1 })

// 形式 2：给更新函数（基于当前 state 算新值）← 更常用
inc: () => set((s) => ({ count: s.count + 1 }))
```

第二种更常用是因为能读当前 state -- 等价于 `set((currentState) => ({ count: currentState.count + 1 }))`。

**initializer 的完整签名**（真实 Zustand）：

```ts
type StateCreator<T> = (
  set: (partial | ((prev: T) => partial)) => void,  // 改 state（第 1 个参数）
  get: () => T,                                       // 读 state（第 2 个参数）
  api: StoreApi<T>,                                   // 完整 API（第 3 个参数）
) => T
```

但 99% 场景只用 `set`，所以平时写 `(set) => ({ ... })` 就够了。需要读当前 state 时用形式 2 的更新函数，或者用 `get`（但 `get` 在 React 组件外才有意义，组件里直接 `useStore(s => s.xxx)` 订阅就行）。

**这就是为什么** `create` 返回的 hook 自动绑死一个 store 实例 -- 它内部就是 `createStore + 闭包`。而 `createStore` 把"绑哪个实例"的决定权交给你（通过 Context 注入）。

**一句话总结**：
- **`create`** = `createStore` + 自动包成 React hook（全局单例场景）
- **`createStore`** = 拿到原始 StoreApi 对象，自己决定怎么注入 React（工厂/多实例场景）
- 两者底层一样，差别只在"谁负责把 store 实例交给 React 组件"

### store 到底把数据存在哪？

**一句话：存在 JS 内存里**。不是 localStorage、不是后端、不是 cookie。

**精确位置**：`createStore` 函数调用时创建的一个 `let state` 局部变量，通过闭包"逃逸"出来，被返回的 StoreApi 对象持有。看极简版：

```ts
function createStore<T>(initializer) {
  let state: T                    // ← 数据就这一个 JS 变量
  const listeners = new Set()

  const api = {
    getState: () => state,        // ← 闭包读取 state
    setState: (next) => {
      state = { ...state, ...partial }  // ← 闭包改写 state
      listeners.forEach((l) => l())
    },
    // ...
  }

  state = initializer(api)
  return api    // ← api 持有对 state 的闭包引用
}
```

**关键点**：`state` 是个普通 JS 变量，本来 `createStore` 函数返回后局部变量会被 GC（垃圾回收），但**返回的 `api` 对象的 `getState/setState` 方法捕获了这个变量的引用**，所以 `state` 跟着 `api` 一起活下来。只要有人持有 `api`（或基于它的 hook），`state` 就不会被回收。

**内存结构图**：

```
浏览器进程内存（JS 堆）
│
├── 模块作用域（counter-store.ts 文件）
│   └── useCounter          ← create 返回的 hook
│       └── 内部持有 store = { getState, setState, subscribe, ... }
│              │
│              └── 闭包捕获 → let state = { count: 0 }  ← 数据在这里
│                            let listeners = Set<...>
│
└── React 组件树
    ├── 组件 A: const count = useCounter(s => s.count)
    │   (注册到 listeners，state 变化时被通知并重渲染)
    └── 组件 B: const inc = useCounter(s => s.inc)
        (拿到 action，调用会触发 store.setState → 改 state → 通知 listeners)
```

**生命周期**：

| 场景 | 何时销毁 |
|---|---|
| `create()` 全局 store | 跟模块一样长，**进程不退出就一直在**。刷新页面 / 关闭 tab 才消失 |
| `createStore()` 工厂 store | 跟持有它的对象一样长。比如 `AgentSessionContext` 的 store，Provider 卸载且没人引用时被 GC |
| React 组件卸载 | 组件不在了，但 store 还在 -- 这是 store 相对 useState 的核心优势 |

**对比：`persist` 中间件会多存一份**

加了 `persist` 的 store，**除了 JS 内存，还多写一份到 localStorage**：

```ts
const usePrefs = create(persist(
  (set) => ({ tocVisible: false }),
  { name: 'my-prefs' },
))
```

| 时机 | 行为 |
|---|---|
| store 创建时 | 从 `localStorage['my-prefs']` 读出来，作为 `state` 初值 |
| 每次 `setState` | 改内存里的 `state` + 序列化写回 `localStorage['my-prefs']` |
| 刷新页面后 | 内存 state 没了，但 localStorage 还在，新 store 用它恢复 |

**所以**：
- 不加 `persist`：数据只在内存里，**刷新页面就丢**
- 加 `persist`：内存 + localStorage 两份，**刷新后从 localStorage 恢复**
- 工厂 store + Context（`AgentSessionContext`）：内存里，**Provider 卸载时一起销毁**

---

## 一句话原则

**简单全局用 `create`，要持久化加 `persist`，多实例用工厂 + Context，按 id 分桶用命名空间 store，UI 即时 + 后端同步用 debounced flush**。

---

## 决策树

```
要跨组件共享一个状态
        │
   刷新页面后还要在吗？
        │
   ┌────┴────┐
   │否        │是
   ▼          ▼
 多个实例各存一份？    后端持久化（跨设备）？
   │                │
   ┌──┴──┐         ┌──┴──┐
   │否    │是       │否    │是
   ▼     ▼         ▼     ▼
 普通全局  命名空间   persist  异步同步
 (1)    分桶 (5)    (2)    (6)
                    │
                    每 workspace 一份？
                    否 -> 普通 persist
                    是 -> 工厂 (3) + Context (4)
```

---

## 6 种用法

### 1. 普通全局 store（最简单）

**核心**：`create((set) => ({...}))` 创建一个挂在模块作用域的 store，所有组件共享。

**最小示例**：
```tsx
const useCount = create<{ count: number; inc: () => void }>((set) => ({
  count: 0,
  inc: () => set((s) => ({ count: s.count + 1 })),
}))
// 用法
const c = useCount((s) => s.count)
```

**项目对照**：
- `web/src/stores/composer-store.ts:26` -- `useComposerStore`，跨面板（文件浏览器等）请求聊天框插入文本
- `web/src/stores/active-session-store.ts:39` -- `useActiveSession`，跨 provider 边界共享"当前哪个 session 被选中"

**适用**：状态全局只有一份、组件卸载后还要在内存里、不需要刷新页面后保留。

---

### 2. 持久化 store（localStorage）

**核心**：`create(persist(...))`，状态自动序列化到 localStorage / sessionStorage，刷新页面后自动恢复。

**最小示例**：
```tsx
const usePrefs = create<{ tocVisible: boolean; setTocVisible: (v: boolean) => void }>()(
  persist(
    (set) => ({
      tocVisible: false,
      setTocVisible: (tocVisible) => set({ tocVisible }),
    }),
    { name: 'my-prefs' },  // localStorage key
  ),
)
```

**项目对照**：
- `web/src/stores/draft-store.ts:14` -- `useDraftStore`，聊天草稿保存到 `tos-chat-drafts`
- `web/src/stores/markdown-preferences-store.ts:9` -- `useMarkdownPreferencesStore`，markdown 目录是否展开

**适用**：单设备偏好、草稿、UI 设置。**不适用**：跨设备同步、敏感数据、超过 5MB 的内容。

---

### 3. 工厂函数 store（每实例一份）

**核心**：不直接 `create`，而是写一个 `createXxxStore()` 函数返回独立 store 实例。每次调用得到新 store。

**最小示例**：
```tsx
import { createStore } from 'zustand/vanilla'  // 注意是 vanilla，不是 React 版
function createChatStore(workspaceId: string) {
  return createStore<State>()(() => ({
    workspaceId,
    messages: [],
    sendMessage: (text) => { /* ... */ },
  }))
}
// 调用方
const store = createChatStore(wsId)
```

**为什么不能直接全局 create**：每个 workspace 有独立的聊天历史，全局一个 store 会把不同 workspace 的消息串在一起。

**项目对照**：
- `web/src/stores/agent-session-store.ts:187` -- `createAgentSessionStore`，每个 workspace 一个；还用 `AgentSessionDeps` 做依赖注入方便单测

### 工厂函数用到的 zustand 接口

工厂函数**不用**普通的 `create`，而是 `zustand/vanilla` 里的 `createStore`，配合 `StoreApi` 类型。三件套：

```ts
import { type StoreApi, createStore } from 'zustand/vanilla'   // vanilla 版
import { useStore } from 'zustand'                              // React hook 版

export function createChatStore(workspaceId: string): StoreApi<ChatSlice> {
  const store = createStore<ChatSlice>()(() => ({
    workspaceId,
    messages: [],
    sendMessage: (text) => { /* ... */ },
  }))
  return store   // 返回 StoreApi 对象，不是 hook
}
```

| API | 来自 | 作用 |
|---|---|---|
| `createStore` | `zustand/vanilla` | 创建 store 对象（非 hook），返回 `StoreApi<T>` |
| `StoreApi<T>` | `zustand/vanilla` (type) | store 的类型，含 `getState/setState/subscribe/getInitialState` |
| `useStore(store, selector)` | `zustand` | React hook，订阅 vanilla store 的切片 |

**`create` vs `createStore` 对比**：

| | `create`（`zustand`） | `createStore`（`zustand/vanilla`） |
|---|---|---|
| 返回值 | React hook `useXxx(selector)` | store 对象 `StoreApi<T>` |
| 能否在 React 外用 | ❌ hook 只能在组件里调 | ✅ 任何地方都能 `getState/setState` |
| 类型参数 | `create<T>()(...)` | `createStore<T>()(initializer)` |
| 适合场景 | 单例全局 store（用法 1/2/5/6） | 工厂 store + Context 注入（用法 3/4） |

**为什么工厂模式必须用 `createStore` 而不是 `create`**：

1. `create` 返回的是**绑死了某个 store 实例的 hook**，但工厂每次调用产出的 store 是新对象 -- 没法预先知道调用方要用哪个实例
2. 工厂要返回 store 对象本身，让调用方决定怎么用（直接 `getState()` 读，或塞进 Context 给子组件订阅）
3. `zustand/vanilla` 没有 React 依赖 -- 后端 / Node 脚本 / 单测里都能直接用，不需要 ReactDOM

**`StoreApi` 在项目里的实际用法**（看 `agent-session-store.ts:187`）：

```ts
export function createAgentSessionStore(
  workspaceId: string,
  deps: AgentSessionDeps,    // 依赖注入：API/SSE/effects 都从外面传
): StoreApi<AgentSessionSlice> {
  const store = createStore<AgentSessionSlice>()(() => ({ ... }))
  return store
}
```

返回的 `StoreApi` 在 `AgentSessionContext.tsx:68` 被 `useState(() => createAgentSessionStore(...))` 接住，塞进 Context；子组件用 `useStore(store, selector)` 订阅切片。

**注意**：工厂 store **不能** 直接 `useStore`，因为 React 没有 store 引用。必须配合下面第 4 种 -- Context 注入。

---

### 4. Context + vanilla store（绑定 React 生命周期）

**核心**：用 `createStore`（来自 `zustand/vanilla`）创建一个**纯 store 对象**（不是 hook），通过 `createContext` + `Provider` 注入给子组件。store 的生命周期跟 Provider 绑定 -- Provider 卸载，store 跟着销毁。

**最小示例**：
```tsx
import { createStore } from 'zustand/vanilla'
import { useStore } from 'zustand'

const Ctx = createContext<StoreApi<State> | null>(null)

function Provider({ children }) {
  // useState 做懒初始化 -- 只创建一次
  const [store] = useState(() => createStore<State>(() => ({ ... })))
  return <Ctx.Provider value={store}>{children}</Ctx.Provider>
}

function useSlice<T>(selector: (s: State) => T): T {
  return useStore(useContext(Ctx)!, selector)
}
```

**为什么用 useState 包**：保证 store 只创建一次（StrictMode 双渲染也不会重复），且跟随 Provider 生命周期。

**项目对照**：
- `web/src/stores/AgentSessionContext.tsx:50` -- `AgentSessionProvider`，把 `createAgentSessionStore` 的产物塞进 Context，让 workspace 页面下所有组件都能订阅消息流

**适用**：配合第 3 种工厂 store，让子组件能用 hook 订阅；或需要把 store 销毁时回收资源（SSE 连接、订阅句柄）。

---

### 5. 命名空间 store（按 id 分桶）

**核心**：一个全局 store，内部按 `Record<workspaceId, Record<instanceId, Record<key, value>>>` 分桶。组件用 `useInstanceState(wsId, instId, key)` 读写自己的桶，互不干扰。

**最小示例**：
```tsx
const useStore = create<{
  byWorkspace: Record<string, Record<string, Record<string, unknown>>>
}>((set) => ({
  byWorkspace: {},
}))

function useInstanceState<T>(wsId: string, instId: string, key: string, def: () => T) {
  return useSyncExternalStore(
    (cb) => useStore.subscribe(cb),
    () => useStore.getState().byWorkspace[wsId]?.[instId]?.[key] ?? def(),
  )
}
```

**为什么不用每实例一个 store**：实例数量可能很多（几十个面板），每个都建一个 store 太重。一个 store + 命名空间分桶，只订阅自己那个 key。

**项目对照**：
- `web/src/stores/instance-state-store.ts:50` -- `useInstanceState`（内存版，刷新丢）
- `web/src/stores/instance-state-store.ts:155` -- `useInstancePersistentState`（持久版，写 workspace_profile 到后端）

**适用**：UI 临时状态（滚动位置、树展开、搜索词）、布局切换要保留的视图状态。

---

### 6. 异步同步 store（本地立刻 set + debounced flush 到后端）

**核心**：调用 `patch()` 时**立刻**更新本地 state（UI 即时响应），同时把 patch 放进 `pending` Map，定时器到了再批量调后端 API。

**最小示例**：
```tsx
const useStore = create<State & Actions>((set) => ({
  byId: {},
  patch(id, patch) {
    set((s) => ({ byId: { ...s.byId, [id]: { ...s.byId[id], ...patch } } }))  // 立刻更新视图
    pending.set(id, { ...pending.get(id), ...patch })
    scheduleFlush(id)  // 400ms 后批量调 API
  },
}))

async function flush(id) {
  const patch = pending.get(id)
  await api.save(id, patch)
  pending.delete(id)
}
```

**为什么不直接调 API**：用户每改一下就发请求会爆。debounce 一下既保证 UI 流畅又保护后端。

**项目对照**：
- `web/src/stores/workspace-profile-store.ts:61` -- `useWorkspaceProfileStore`，400ms debounce 写到 `PATCH /workspaces/:id/profile`
- `web/src/stores/fleet-profile.ts:39` -- 注册一个 backend override，让 fleet 范围用 `user_profile` API 而不是 workspace profile

**适用**：UI 需要立刻响应、但持久化目标是远端后端、且改动可能频繁的场景。

---

## 选型决策表

| 需求场景 | 选哪种 | 项目对照文件 |
|---|---|---|
| 几个不相关组件共享一个值 | 1 普通全局 | `composer-store.ts` |
| 刷新页面后保留（单设备） | 2 persist | `draft-store.ts` |
| 每个 workspace/页面有独立实例 | 3 工厂 + 4 Context | `agent-session-store.ts` + `AgentSessionContext.tsx` |
| 多 instance 共享一个 store，按 id 分桶 | 5 命名空间 | `instance-state-store.ts` |
| UI 即时响应 + 后端批量写入 | 6 异步同步 | `workspace-profile-store.ts` |
| 状态跨设备同步（不只是 localStorage） | 6 异步同步 | `workspace-profile-store.ts` |

---

## 常见坑

1. **selector 返回新引用会无限重渲染**
   ```tsx
   // ❌ 每次都返回新数组
   const items = useX((s) => s.items.filter(i => i.active))
   // ✅ 用 useShallow 或把派生放组件里
   const items = useShallow(useX((s) => s.items.filter(i => i.active)))
   ```

2. **不要在 render 期间调 set**：突变只在事件处理函数或 `useEffect` 里做，render 期间调会触发额外渲染。

3. **工厂 store 不配 Context 会泄漏**：`createStore` 出来的实例如果不绑生命周期，会一直留在内存里。必须用第 4 种 Context 模式，Provider 卸载时 store 也销毁（`destroy()` 清订阅）。

4. **persist 不是数据库**：localStorage 有 5MB 限制、单设备、用户能手动清。需要跨设备就用第 6 种 -- 写后端。

5. **命名空间 store 注意 selector 粒度**：订阅整个 `byWorkspace` 会让任何 key 变化都触发重渲染。用 `useSyncExternalStore` 精确订阅到 `(wsId, instId, key)` 那一格。

---

## 一句话总结

> store = 跨组件、跨生命周期、跨边界的可变状态。本仓库 6 种用法从最简到最复杂：**普通全局 → persist → 工厂 → Context 注入 → 命名空间分桶 → 异步同步**，按"刷新要不要保留、要不要多实例、UI 是不是要立刻响应"三个问题选型。

---

## 文件索引

| 文件 | 用法 | 导出 |
|---|---|---|
| `composer-store.ts` | 1 普通全局 | `useComposerStore`, `useComposerInsertRequests` |
| `active-session-store.ts` | 1 普通全局 | `useActiveSession`, `useSessionNavigation` |
| `markdown-preferences-store.ts` | 2 persist | `useMarkdownPreferencesStore` |
| `draft-store.ts` | 2 persist | `useDraftStore`, `useDraft`, `getDraftFor` 等 |
| `agent-session-store.ts` | 3 工厂 | `createAgentSessionStore` |
| `AgentSessionContext.tsx` | 4 Context + 工厂 | `AgentSessionProvider`, `useAgentSessionStore` |
| `instance-state-store.ts` | 5 命名空间（内存 + 持久双变体） | `useInstanceState`, `useInstancePersistentState` |
| `workspace-profile-store.ts` | 6 异步同步 | `useWorkspaceProfileStore`, `useWorkspaceProfile` |
| `fleet-profile.ts` | 6 异步同步（backend override） | `FLEET_PROFILE_ID`（注册副作用） |
