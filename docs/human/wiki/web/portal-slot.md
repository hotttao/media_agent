# portal-slot.md - useAppHeaderSlot + createPortal 怎么协作

> 讲清楚 `WorkspaceFilesPanel.tsx` 里 `useAppHeaderSlot()` + `createPortal(...)` 这对组合为什么存在、怎么工作。读完应该能回答："一个面板组件怎么把自己的按钮渲染到外层 AppWindow 的头部里？为什么不用普通 props 传 children？"

---

## 一句话讲清

**`useAppHeaderSlot` 拿到外层 AppWindow 头部的 DOM 节点（通过 React Context），`createPortal` 把当前组件的 JSX 渲染到那个 DOM 节点里**。两者组合 = "动态插槽"模式 -- 让内层组件在不改外层 API 的前提下，把内容塞进外层布局的指定位置。

---

## 痛点：为什么不用普通 props

假设 AppWindow 想让面板组件自定义头部内容，最直觉的写法：

```tsx
<AppWindow headerContent={<MyCustomHeader />} />
```

但项目里 `AppWindow` 是个**通用容器**，面板组件（Files、Chat、Settings...）放在它的 `children` 里渲染：

```tsx
<AppWindow>
  <WorkspaceFilesPanel />   {/* 这个组件想往 AppWindow 头部塞按钮 */}
</AppWindow>
```

如果用 props，面板组件拿不到 AppWindow 实例 -- 它在 children 里，不能反向影响外层布局。两种解法：

| 方案 | 问题 |
|---|---|
| 用 render props / children-as-function | API 复杂、类型推导难、嵌套丑 |
| **Context 暴露 DOM + createPortal 注入** ✅ | 面板组件自己决定渲染什么、什么时候渲染 |

---

## 拆解 1：`useAppHeaderSlot` -- 通过 Context 暴露 DOM 节点

看 `web/src/components/shell/windows/AppWindow.tsx:48-52`：

```tsx
export const AppHeaderSlotContext = createContext<HTMLDivElement | null>(null)

export function useAppHeaderSlot(): HTMLDivElement | null {
  return useContext(AppHeaderSlotContext)
}
```

AppWindow 在自己的 header 里放了一个 `<div>` 当"插槽"，用 `useState` 把它的 DOM 节点存起来：

```tsx
// AppWindow.tsx:69
const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null)

// AppWindow.tsx:109
<header>
  <div ref={setHeaderSlot} className="flex min-w-0 flex-1 items-center gap-1.5" />
  <WindowControls ... />
</header>

// AppWindow.tsx:122 -- 通过 Context 把这个 DOM 暴露给所有子组件
<AppHeaderSlotContext.Provider value={headerSlot}>
  <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
</AppHeaderSlotContext.Provider>
```

**关键点**：
- `ref={setHeaderSlot}` -- React 把 div 挂载后，把 DOM 节点塞进 `headerSlot` state
- 用 `useState` 而不是 `useRef` -- **state 变化触发重渲染**，子组件第一次渲染时 `headerSlot = null`，DOM 挂载后 state 更新，子组件重新订阅到非空值，能正确 portal
- Provider 把 `headerSlot` 暴露给 children 树 -- 任何子组件调 `useAppHeaderSlot()` 都能拿到

---

## 拆解 2：`createPortal` -- 跨 React 树渲染

React 默认按组件树渲染到 DOM。`createPortal(reactNode, domNode)` 让你**把 reactNode 渲染到任意 DOM 节点**，跳过组件树的物理位置约束：

```tsx
import { createPortal } from 'react-dom'

createPortal(
  <MyButtons />,    // 要渲染的 JSX
  headerSlot,        // 目标 DOM 节点（来自 useAppHeaderSlot）
)
```

**关键点**：
- **DOM 上**：JSX 出现在 `headerSlot` 这个 div 里（即 AppWindow 的 header）
- **React 事件上**：JSX 还属于当前组件的 React 子树，事件冒泡、Context 访问、错误边界都跟当前组件走，**不会**被 AppWindow 错误捕获

这就是 portal 的精髓：**DOM 在这里，事件在那里**。

---

## 两者结合：动态插槽模式

`WorkspaceFilesPanel.tsx:121` 拿到 slot，`:1090` 用 portal 渲染：

```tsx
// 第 1 步：拿 slot DOM
const headerSlot = useAppHeaderSlot()

// 第 2 步：渲染时，如果 slot 存在，把按钮 portal 进去
return (
  <>
    {headerSlot &&
      createPortal(
        <>
          <Tabs ...>...</Tabs>           {/* drive 切换 */}
          <AppHeaderButton icon={Search} ... />
          <AppHeaderButton icon={Upload} ... />
          <AppHeaderButton icon={FolderPlus} ... />
          {/* 面包屑 */}
          <div className="flex min-w-0 flex-1 ...">...</div>
        </>,
        headerSlot,
      )}

    {/* 面板自身的 body 内容 -- 不在 header 里 */}
    <div className="relative flex h-full flex-col">
      {/* 文件列表 / FileViewer / 上传进度条 / ... */}
    </div>
  </>
)
```

**完整流程**：

```
AppWindow 渲染
  ↓
  <header>
    <div ref={setHeaderSlot} />       ← DOM 节点 X 创建
    <WindowControls />                  ← 窗口控制按钮
  </header>
  <AppHeaderSlotContext.Provider value={headerSlot}>  ← state 还是 null
    <WorkspaceFilesPanel />
  </AppHeaderSlotContext.Provider>
  ↓
React 挂载完成，调 ref 回调 -> setHeaderSlot(节点 X)
  ↓
state 变化 -> AppWindow 重渲染 -> Provider value 变成节点 X
  ↓
WorkspaceFilesPanel 重渲染 -> useAppHeaderSlot() 返回节点 X
  ↓
createPortal(<buttons />, 节点 X)
  ↓
节点 X 里出现了按钮 -- DOM 上 AppWindow header 显示这些按钮
事件上这些按钮还属于 WorkspaceFilesPanel 的 React 子树
```

---

## 项目里的实战：WorkspaceFilesPanel 的 5 段式 header

`WorkspaceFilesPanel.tsx:1090-1274` 的 header portal 内容根据状态切换 5 种形态：

| 状态 | header 渲染什么 | 代码位置 |
|---|---|---|
| 目录列表（默认） | drive 切换 + 6 个动作按钮 + 面包屑 | `:1124-1183` + `:1204-1259` |
| 搜索中 | drive 切换 + 6 个动作按钮 + 搜索 Input | `:1124-1183` + `:1188-1202` |
| 看文件 | (无 drive 切换) + Refresh + FileViewer 注入的按钮 + 复制路径 | `:1116-1123` + `:1260-1271` |
| host 锁定 drive | (无 drive 切换) + 6 个动作按钮 + 面包屑 | `:1096` 的 `!lockedDrive` 判断 |
| AFS root | (无 drive 切换) + 6 个动作按钮 + (无面包屑) | 同上 |

**关键技巧**：所有这些分支都通过 `createPortal(<>...</>, headerSlot)` 一次 portal 完成，外层 AppWindow 不需要知道面板在画什么。

---

## 嵌套 portal：FileViewer 二级插槽

`WorkspaceFilesPanel` 不只消费 `useAppHeaderSlot`，它**自己也开了一个插槽**让子组件 `FileViewer` 往里塞按钮。这是模式的两层嵌套：

### 第 1 层：WorkspaceFilesPanel → AppWindow header

```tsx
// WorkspaceFilesPanel.tsx:121
const headerSlot = useAppHeaderSlot()
// :1090
{headerSlot && createPortal(<...>, headerSlot)}
```

### 第 2 层：FileViewer → WorkspaceFilesPanel 的"文件动作插槽"

WorkspaceFilesPanel 在自己 portal 出去的 header 区域里**再开一个 div 当二级插槽**：

```tsx
// WorkspaceFilesPanel.tsx:375 -- 创建二级插槽
const [fileActionsSlot, setFileActionsSlot] = useState<HTMLDivElement | null>(null)

// :1116 -- 在 portal 出去的 header 里挂这个插槽
{isViewingFile ? (
  <div ref={setFileActionsSlot} className="flex shrink-0 items-center gap-0.5">
    <AppHeaderButton icon={RefreshCw} onClick={refresh} />
    {/* FileViewer 会 portal 它的按钮到这里 */}
  </div>
) : ...}

// :1337 -- 把插槽 DOM 作为 prop 传给 FileViewer
<FileViewer
  headerSlot={fileActionsSlot}
  ...
/>
```

FileViewer 拿到这个 prop 后，用同样的方式 `createPortal(<buttons />, headerSlot)` 把自己的"下载 / 复制 / 编辑 / 重命名"按钮塞进去。

### 为什么要两层

- **第 1 层**：WorkspaceFilesPanel 自己的按钮（drive 切换、面包屑、搜索等）要进 AppWindow header
- **第 2 层**：FileViewer 的按钮**也要进同一个 AppWindow header**，但 FileViewer 不直接知道 AppWindow（它是 WorkspaceFilesPanel 的子组件）-- 通过 WorkspaceFilesPanel 转交

这就是 portal-slot 模式的扩展性：**任何层级的组件都能往上层 header 里塞内容**，只要沿着链路把 slot DOM 传下去。

---

## 关键点 + 常见坑

### 1. 为什么用 `useState` 存 DOM 而不是 `useRef`

```tsx
// ❌ useRef：first render 时 headerSlot.current = null，子组件拿到 null，portal 失败
const headerSlot = useRef<HTMLDivElement>(null)
<div ref={headerSlot} />

// ✅ useState：DOM 挂载后调 setState，触发重渲染，子组件第二次拿到真实 DOM
const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null)
<div ref={setHeaderSlot} />
```

`useRef` 的 mutation 不触发重渲染，子组件就永远是 `null`。**Portal slot 必须用 state**。

### 2. 必须 `headerSlot && createPortal(...)` 守卫

```tsx
// ❌ 第一次渲染时 headerSlot 是 null，createPortal(jsx, null) 会报错
{createPortal(<Buttons />, headerSlot)}

// ✅ 守卫，等 DOM 挂载完再 portal
{headerSlot && createPortal(<Buttons />, headerSlot)}
```

### 3. 事件还在原 React 树里

```tsx
// WorkspaceFilesPanel 把按钮 portal 到 AppWindow header
createPortal(<AppHeaderButton onClick={handleRefresh} />, headerSlot)

// 但 onClick 的 handler 还在 WorkspaceFilesPanel 闭包里
// handleRefresh 能读到 workspaceId、drive、currentPath 等 WorkspaceFilesPanel 的 state
```

如果 portal 出去的组件需要 Context（比如主题、i18n），它**仍能访问原 React 树的 Context**，不会断。这是 portal 相对于"用 DOM API 手动 appendChild"的关键优势。

### 4. 空状态也要 portal

```tsx
// ❌ 条件渲染整个 portal -- 切换时整个 header 闪烁
{showButtons && headerSlot && createPortal(<Buttons />, headerSlot)}

// ✅ 始终 portal，让内部条件决定渲染什么 -- 切换更平滑
{headerSlot && createPortal(showButtons ? <Buttons /> : <EmptyState />, headerSlot)}
```

WorkspaceFilesPanel 就是这么做的：始终 portal，内部用 `isViewingFile ? ... : ...` 切换内容。

---

## 顺便讲讲 `data-state` 的用法（CSS 用法）

`WorkspaceFilesPanel.tsx:1129` 出现了 `data-state`：

```tsx
<AppHeaderButton
  icon={Search}
  data-state={isSearching ? 'open' : undefined}
  title={t('components.workspaceFiles.actions.search')}
  onClick={() => { /* ... */ }}
/>
```

这是什么、为什么要这样写？

### `data-state` 是什么

HTML 的**自定义属性**，前缀 `data-`。浏览器不认识它，**专门给 CSS 和 JS 用**。可以写任何值：

```html
<button data-state="closed">搜索</button>
<!-- 搜索展开后变成 -->
<button data-state="open">搜索</button>
```

### CSS 用"属性选择器"响应它

```css
[data-state="open"] { background: blue; }   /* 搜索展开时按钮变蓝 */
```

Tailwind 写法：`data-[state=open]:bg-foreground/[0.06]`。看 `AppHeaderButton.tsx:43` 末尾的 `data-[state=open]:bg-foreground/[0.06] data-[state=open]:text-foreground` -- 这是 `AppHeaderButton` 组件自己定义的样式，**任何**用它的人传 `data-state="open"`，按钮自动变蓝。

### 为什么不用条件 className

```tsx
// ❌ 传统：条件 className -- 每个调用点都要自己拼样式
<AppHeaderButton
  className={isSearching ? 'bg-foreground/[0.06] text-foreground' : ''}
  icon={Search}
/>

// ✅ data-state：值写到 DOM 上，CSS 自己决定要不要响应
<AppHeaderButton
  data-state={isSearching ? 'open' : undefined}
  icon={Search}
/>
```

**`data-state` 的优势**：

| 维度 | 优势 |
|---|---|
| **语义化** | `data-state="open"` 在 DOM 上一眼能看出组件状态，调试方便 |
| **解耦** | JS 只管"标状态"，CSS 管"画样式"，可以独立修改 |
| **跟 Radix 对齐** | Radix 的 `DropdownMenuTrigger` / `PopoverTrigger` 等都用 `data-state="open/closed"` 标开关状态 -- 业务组件用同样的属性，hover / active / open 都用同一套 CSS，**视觉一致** |
| **复用** | `AppHeaderButton` 内部已经写好对 `data-[state=open]` 的响应，调用方传 `data-state="open"` 就能拿到统一的开态样式，不用每处重写 |

### `undefined` vs 字符串

```tsx
data-state={isSearching ? 'open' : undefined}
```

- 写 `'open'`：DOM 是 `<button data-state="open">`，CSS `[data-state="open"]` 匹配，按钮变蓝
- 写 `undefined`：React 不渲染这个属性，DOM 是 `<button>`，CSS 不匹配，回到默认样式
- 写 `'closed'`：DOM 是 `<button data-state="closed">`，CSS 可以专门为关闭态写样式（Radix 就这么用）

WorkspaceFilesPanel 这里只关心 "open" 一个状态，关闭时直接 `undefined` 最省事 -- 不用为 `[data-state="closed"]` 写一条空规则。

### 跟 Radix 协作的完整闭环

当你写：

```tsx
<DropdownMenuTrigger asChild>
  <AppHeaderButton icon={MoreHorizontal} />
</DropdownMenuTrigger>
```

Radix 内部会**自动**给 `AppHeaderButton` 注入 `data-state="open"` 或 `data-state="closed"`（看菜单是否展开），AppHeaderButton 的 `:43` 行 Tailwind class 就响应它 -- 这就是为什么同一个 `AppHeaderButton` 既能单独用 `data-state` 标搜索态，又能被 Radix 包成菜单触发器自动响应开关态。

> 关于 Radix 怎么注入 `data-state`，看 `radix.md`。

---

## 一句话总结

> **`useAppHeaderSlot`** = 从 AppWindow 的 Context 拿到 header 里那个 `<div>` 的 DOM 节点（用 `useState + ref` 存，触发重渲染让子组件拿到非空值）。
>
> **`createPortal(jsx, domNode)`** = 把 JSX 渲染到任意 DOM 节点 -- DOM 上出现在那里，React 事件上还属于当前组件树。
>
> 两者组合 = **动态插槽模式**：内层组件在不改外层 API 的前提下，把内容塞进外层布局的指定位置。WorkspaceFilesPanel 用这个模式往 AppWindow header 塞按钮，并自己也开了一个二级插槽 `fileActionsSlot` 让 FileViewer 继续往里塞 -- 模式可以沿组件树任意嵌套。
