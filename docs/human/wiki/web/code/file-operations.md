# file-operations.md - Dialog 弹窗 + DialogStackContext 的关系

> 讲清楚 `NewFileDialog` 里用的 Radix `<Dialog>` 跟 `web/src/contexts/DialogStackContext.tsx` 是什么关系、为什么代码看起来这么复杂。读完应该能回答："Dialog 弹窗的 8 个组件都是干嘛的？DialogStackContext 跟 Radix Dialog 是什么关系？NewFileDialog 用的是哪一套？"

---

## 一句话讲清

**Radix `<Dialog>` 和 `DialogStackContext` 解决两个不同层级的问题：**

- **Radix `<Dialog>`**：底层 UI 组件，管"一个对话框怎么显示、怎么关、a11y、动画"。每个 dialog 实例自己持有 `open` state。
- **`DialogStackContext`**：上层调度器，管"哪些对话框被打开、按 key 找组件渲染"。它**内部用的也是 Radix Dialog**，只是在上面套了一层"全局注册表 + 栈"。

`NewFileDialog` **只用 Radix Dialog，没用 DialogStackContext** -- 它是"本地对话框"，只在 WorkspaceFilesPanel 内部触发，不需要全局调度。

---

## Dialog 弹窗代码为什么看起来复杂

看 `web/src/components/ui/dialog.tsx` -- 这是 shadcn 风格的封装，把 Radix 的一个 Dialog 拆成 **8 个小组件**：

```
Dialog              ← Radix Root，管 open state
DialogTrigger       ← 点了打开 dialog 的按钮
DialogPortal        ← 把内容 portal 到 body 末尾（避免被父级裁剪）
DialogOverlay       ← 半透明黑色背景层
DialogContent       ← 中间的白色卡片
DialogHeader        ← 卡片顶部（标题区）
DialogFooter        ← 卡片底部（按钮区）
DialogTitle         ← 标题文字
DialogDescription    ← 描述文字
```

每个都是一行 `<DialogPrimitive.Xxx className=... {...props} />`，**唯一的活就是拼 Tailwind class**。看 `dialog.tsx:31-63` 的 `DialogContent` -- 30 多行 class 字符串拼出"居中、圆角、阴影、动画"，没别的逻辑。

所以 `NewFileDialog` 写成：

```tsx
<Dialog open={open} onOpenChange={onOpenChange}>
  <DialogContent>
    <DialogHeader><DialogTitle>...</DialogTitle></DialogHeader>
    <Input ... />
    <DialogFooter>
      <Button>取消</Button>
      <Button>创建</Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

嵌套深，但每层职责单一：`Dialog` 管 open，`DialogContent` 是卡片容器，`DialogHeader/Footer` 是布局，`DialogTitle` 是文字。**这是 shadcn 的"组合式 API"风格，比一个巨型 `<Dialog title=... footer=...>` 灵活**。

---

## DialogStackContext 是干嘛的

看 `web/src/contexts/DialogStackContext.tsx`，核心三块：

### 1. 注册表 `SYNC_REGISTRY`（`:16-20`）

```tsx
const SYNC_REGISTRY: Record<string, ComponentType<DialogProps>> = {}

export function registerDialog(key: string, component: ComponentType<DialogProps>) {
  SYNC_REGISTRY[key] = component
}
```

启动时 `web/src/components/dialogs/registry.ts` 调用 9 次 `registerDialog`，把 `'create-workspace'` -> `CreateWorkspaceDialog` 这样的映射塞进去。**这是全局字典**，任何地方拿 key 都能查到组件。

`registry.ts` 的内容：

```ts
registerDialog('create-provider', CreateProviderDialog)
registerDialog('create-credential', CreateCredentialDialog)
registerDialog('create-environment', CreateEnvironmentDialog)
registerDialog('create-token', CreateTokenDialog)
registerDialog('create-connector', CreateConnectorDialog)
registerDialog('create-skill', CreateSkillDialog)
registerDialog('create-template', CreateTemplateDialog)
registerDialog('create-workspace', CreateWorkspaceDialog)
registerDialog('preferences', PreferencesDialog)
```

### 2. Context 暴露三个方法（`:25-37`）

```tsx
interface DialogStackContextValue {
  open: (key: DialogKey) => void    // 把 key 加进 activeDialogs
  close: (key: DialogKey) => void  // 从 activeDialogs 删 key
  isOpen: (key: DialogKey) => boolean
}
```

任何组件 `useDialogStack()` 拿到这三个方法，就能开/关对话框，**不用自己 mount 对话框组件**。

调用方例子（`WorkspacesApp.tsx:40`、`CommandPalette.tsx:299` 等）：

```tsx
const { open: openDialog } = useDialogStack()
// ...
onClick={() => openDialog('create-workspace')}
```

调用方**完全不需要**在 JSX 里写 `<CreateWorkspaceDialog open={...} />`，Provider 会自动渲染。

### 3. Provider 渲染当前打开的对话框（`:57-74`）

```tsx
<DialogStackContext.Provider value={...}>
  {children}
  {Array.from(activeDialogs).map((key) => {
    const Component = SYNC_REGISTRY[key]  // 按 key 拿组件
    if (!Component) return null
    return <Component key={key} open onOpenChange={(v) => { if (!v) closeDialog(key) }} />
  })}
</DialogStackContext.Provider>
```

关键：**Provider 自己负责渲染所有打开的对话框**，挂在 `children` 后面。这就是"栈"模式 -- `activeDialogs` 是个 `Set<string>`，可以同时开多个对话框。

Provider 挂在 `Desktop.tsx:149` 的最外层，整个 app 都在它的 children 里：

```tsx
<DialogStackProvider>
  {/* 整个 app */}
</DialogStackProvider>
```

---

## 两套系统的对比

| 维度 | Radix Dialog 直接用 | DialogStackContext |
|---|---|---|
| 谁控制 open | 调用方用 `useState` 自己管 | Provider 用 `Set<string>` 集中管 |
| 谁渲染 Dialog | 调用方在 JSX 里手动 `<Dialog>` | Provider 按 key 从 registry 拿组件渲染 |
| 触发位置 | 必须能拿到 dialog 组件实例 | 任何地方 `useDialogStack().open('create-workspace')` |
| 适合场景 | 局部对话框（只在某个面板里用） | 全局对话框（命令面板、markdown 链接、任意按钮都可能触发） |

### 项目里的实际划分

**用 DialogStackContext 的**（在 `registry.ts` 里注册）：

- `create-workspace` / `create-provider` / `create-credential` / `create-environment`
- `create-token` / `create-connector` / `create-skill` / `create-template`
- `preferences`

这些都是"全局可触发"的，命令面板（`CommandPalette.tsx:299`）、左侧导航（`WorkspacesApp.tsx:40`）、WsSwitcher（`WsSwitcher.tsx:39`）、各个 management 页面都能打开它们。

**直接用 Radix Dialog 的**（不在 registry 里）：

- `NewFileDialog` / `NewDirDialog` / `RenameDialog`
- 上传进度条等

这些都只在 `WorkspaceFilesPanel` 内部用，外部不会触发。

---

## NewFileDialog 跟 DialogStackContext 的关系

**没有直接关系**。具体来说：

1. `NewFileDialog` 用 Radix `<Dialog>`（`file-operations.tsx:663`），open state 由 `WorkspaceFilesPanel.tsx:341` 的 `useState(false)` 管
2. `NewFileDialog` **没有**在 `registry.ts` 里 `registerDialog` -- 你可以看 `registry.ts` 全文，里面没有它
3. `WorkspaceFilesPanel` **没有**调 `useDialogStack()` -- 它直接 `setNewFileOpen(true)`

### 为什么 NewFileDialog 故意不注册

如果未来要让命令面板也能打开"新建文件"对话框，就要：

1. 在 `registry.ts` 加一行 `registerDialog('new-file', NewFileDialog)`
2. `WorkspaceFilesPanel` 改用 `useDialogStack().open('new-file')`
3. 但 `NewFileDialog` 需要 `parentDir` 参数 -- `DialogProps` 只允许 `{ open, onOpenChange }`，要么扩展契约，要么用闭包/额外 Context 传 parentDir

这就是为什么 NewFileDialog **故意没注册** -- 它的入参（parentDir）跟 DialogStackContext 的"无参 open(key)"契约不匹配，硬塞进去会污染 registry 的统一接口。

`CreateWorkspaceDialog` 这类能注册，是因为它们"打开就是新建一条记录"，不需要业务参数；而 `NewFileDialog` 必须知道"在哪个目录下建"，参数化困难。

---

## 一句话总结

> Radix `<Dialog>` 是底层 UI 组件，每个 dialog 实例自己管 open state；`DialogStackContext` 是上层调度器，用 `Set<string>` + `registry` 集中管理"哪些 key 打开 + 每个 key 对应哪个组件"，让任何组件都能 `openDialog(key)` 触发。
>
> `NewFileDialog` 只用 Radix Dialog，**没注册到 DialogStackContext** -- 因为它只在 WorkspaceFilesPanel 内部用，且需要 `parentDir` 参数，跟 stack 的"无参 open(key)"契约不匹配。registry 里全是"无参可触发"的全局对话框（create-workspace 等）。
