# WorkspaceFilesPanel.md - 页面结构 + Upload 流程

> 这个文档讲两件事：
> 1. **页面结构**：`WorkspaceFilesPanel.tsx` 渲染出来的 UI 长什么样、每个区域怎么布局 -- 配 ASCII 结构图。
> 2. **Upload 流程**：`WorkspaceFilesPanel.tsx:1139` 的 `<AppHeaderButton icon={Upload} onClick={() => fileInputRef.current?.click()} disabled={!writeAllowed} />` 被点击后到底发生了什么。

---

## 第一部分：页面结构

`WorkspaceFilesPanel` 渲染在 `AppWindow`（通用窗口容器）的 children 里。先看外层，再看内部。

### 外层 AppWindow（`AppWindow.tsx:90-124`）

```
┌────────────────────────────────────────────────────────────────┐
│ <section>  flex flex-col h-full                               │ 外框
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ <header>  flex h-9 shrink-0           ← 固定 36px 高       │ │
│ │  ┌────────────────────────────────┐  ┌───────────┐         │ │
│ │  │ <div ref={setHeaderSlot}>      │  │ Window    │         │ │
│ │  │   flex flex-1 min-w-0           │  │ Controls  │         │ │
│ │  │   ← WorkspaceFilesPanel 把按钮 │  │ shrink-0 │         │ │
│ │  │     portal 到这里              │  │           │         │ │
│ │  └────────────────────────────────┘  └───────────┘         │ │
│ └────────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ <div>  flex flex-1 flex-col min-h-0   ← body 容器         │ │
│ │   {children} ← WorkspaceFilesPanel                         │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

**布局要点**：

- `<section>` 用 `flex flex-col h-full` 把窗口竖向分成 header + body 两段
- `<header>` 用 `h-9 shrink-0` 固定 36px 不缩，里面 `flex` 横向排：slot 占 `flex-1` + WindowControls `shrink-0` 永远靠右
- body `<div>` 用 `flex-1 min-h-0` 占满剩余高度 -- **`min-h-0` 是关键**：Flexbox 默认子内容不缩，会导致 ScrollArea 滚不动，必须手动加

### WorkspaceFilesPanel 内部结构（`:1088-1641`）

```tsx
return (
  <>
    {/* 1. header portal - 渲染到 AppWindow header slot */}
    {headerSlot && createPortal(<HeaderContent />, headerSlot)}

    {/* 2. body 主体 */}
    <div className="relative flex h-full flex-col">
      {isExternalDragging && <DragOverlay />}      {/* 2a 拖拽浮层 */}
      <input className="hidden" />                   {/* 2b 隐藏 input */}
      {uploading && <UploadProgress />}              {/* 2c 进度条 */}
      {isViewingFile ? <FileViewer />               {/* 2d 内容区 - 5 选 1 */}
       : afsRoot ? <AfsSharesPanel />
       : error ? <Alert />
       : isLoading ? <Spinner />
       : empty ? <EmptyHero />
       : <FileList />}

      <NewFileDialog />                              {/* 2e Dialog × 5 */}
      <NewFolderDialog />
      <RenameDialog />
      <PublicLinkDialog />
      <ManagePublicLinksDialog />
    </div>
  </>
)
```

### 完整页面布局（默认文件列表模式）

```
┌────────────────────────────────────────────────────────────────┐
│ AppWindow header (36px)  ← headerSlot portal 内容              │
│  [Workspace|AFS] [🔍][⬆][📄][📁][🔗][🔄]  workspace > folder ›│
├────────────────────────────────────────────────────────────────┤
│ body: flex flex-col h-full                                     │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ Upload Progress (仅上传时, shrink-0)                      │ │
│ ├────────────────────────────────────────────────────────────┤ │
│ │ 列头 / 选中条  (sticky top-0 h-8)                         │ │
│ │  名称                        修改时间        大小          │ │
│ ├────────────────────────────────────────────────────────────┤ │
│ │ ScrollArea (flex-1)                                        │ │
│ │  📁 Documents                        2小时前       -       │ │
│ │  ────────────────────────────────────────────────────       │ │
│ │  📄 report.pdf                     昨天 14:32      2M      │ │
│ │  ────────────────────────────────────────────────────       │ │
│ │  📄 photo.jpg                      3天前 11:08     5M      │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

### 各区域布局说明

| 区域 | 布局作用 | 关键 class |
|---|---|---|
| Header portal | 横向排：drive 切换 + 6 按钮 + 面包屑（占满剩余宽度） | `shrink-0` 按钮 / `flex-1 min-w-0` 面包屑 |
| Body 容器 | 竖向 flex，相对定位（给拖拽浮层做参考） | `relative flex flex-col h-full` |
| 拖拽浮层 | 覆盖整个 body，浮在内容上，不拦截鼠标 | `absolute inset-2 z-10 pointer-events-none` |
| 上传进度条 | 高度固定不被压缩 | `shrink-0` |
| 列头/选中条 | 滚动时吸顶 | `sticky top-0 z-[1] h-8` |
| ScrollArea | 占满剩余高度，超出滚动 | `flex-1` |

### 文件列表的行布局（`:1477-1585`）

每行 4 列，名称占满剩余宽度，其他列固定宽度：

```
[图标] [名称 flex-1 min-w-0]  [修改时间 w-24]  [大小 w-16]  [菜单 w-6]
 4px      占满 + truncate        96px         64px         24px
```

- 名称列用 `flex-1 min-w-0` + `truncate` -- 占满 + 超长省略
- 时间/大小列用 `shrink-0 w-N` -- 固定列宽不被压缩
- 选中态：左侧 `absolute` 2px 竖条 + 主色淡背景

### 核心布局模式总结

整个面板就 3 种 Flexbox 模式反复用：

| 模式 | class | 解决什么 |
|---|---|---|
| 竖向 flex + `min-h-0` | `flex flex-col min-h-0 flex-1` | 让嵌套层级的 ScrollArea 能滚 |
| 横向 flex + `flex-1 min-w-0` | `flex flex-1 min-w-0` | 占满剩余宽度 + 允许截断 |
| 固定宽度列 + `shrink-0` | `shrink-0 w-N` | 列宽固定不被压缩 |

外加 `sticky top-0 z-[1]` 做吸顶列头，`absolute inset-2 z-10` 做拖拽浮层。

### Dialog 不在 DOM 树里

5 个 Dialog（NewFile / NewFolder / Rename / PublicLink / ManagePublicLinks）是 Radix Dialog，内部用 `<DialogPortal>` 渲染到 `document.body` 末尾，**不在 WorkspaceFilesPanel 的 DOM 树里**。默认 `open=false` 不挂载，点按钮才 `setXxxOpen(true)` 显示。

---

## 第二部分：Upload 按钮点击后的执行过程

> 下面讲 `WorkspaceFilesPanel.tsx:1139` 的 `<AppHeaderButton icon={Upload} onClick={() => fileInputRef.current?.click()} disabled={!writeAllowed} />` 被点击后到底发生了什么。

---

## 一句话讲清

**点击 AppHeaderButton 不直接上传文件，而是编程触发隐藏的 `<input type="file">` 让浏览器弹文件选择框；用户选完文件触发 `onChange` -> `handleUpload` -> 对每个文件起一个 XMLHttpRequest (XHR) 上传，过程中通过 `xhr.upload.onprogress` 回调更新 `uploads` Map state -> React 重新渲染进度条；全部完成后调 `invalidateQueries` 让 React Query 重新拉文件列表。**

5 步流程：

```
1. 用户点 AppHeaderButton (Upload 图标)
     ↓ onClick: fileInputRef.current?.click()
2. 隐藏的 <input type="file"> 被编程触发
     ↓ 浏览器弹文件选择框
3. 用户选文件 -> input.onChange 触发
     ↓ 调 handleUpload(files)
4. handleUpload 对每个文件起一个 uploadFile (XHR)
     ↓ xhr.upload.onprogress 持续更新 uploads Map
     ↓ React 重渲染 -> 进度条动
5. 所有 XHR 完成 -> Promise.all 解决 -> invalidateListing
     ↓ React Query 重新拉列表 -> 文件出现
```

---

## 逐步拆解

### T0：用户点击 AppHeaderButton

`WorkspaceFilesPanel.tsx:1139-1148`：

```tsx
<AppHeaderButton
  icon={Upload}
  title={writeAllowed ? t('...upload') : t('...uploadDisabled')}
  onClick={() => fileInputRef.current?.click()}
  disabled={!writeAllowed}
/>
```

关键点：

- `disabled={!writeAllowed}` -- 只读 drive 下按钮禁用，`onClick` 不会触发
- `onClick={() => fileInputRef.current?.click()}` -- 点击时**调 `fileInputRef.current`**（隐藏 input 的 DOM 节点）的 `.click()` 方法
- `?.` 是可选链，第一次渲染时 `fileInputRef.current` 可能还是 `null`，避免 `null.click()` 报错

### T1：编程触发隐藏 input

`WorkspaceFilesPanel.tsx:1296-1305` 里有一个用户看不见的 `<input>`：

```tsx
<input
  ref={fileInputRef}
  type="file"
  multiple
  className="hidden"
  onChange={(e) => {
    if (e.target.files) handleUpload(e.target.files)
    e.target.value = ''
  }}
/>
```

关键点：

- `className="hidden"` -- Tailwind 的 `display: none`，用户看不见
- `type="file"` -- 浏览器内置的文件选择控件
- `multiple` -- 允许选多个文件
- `ref={fileInputRef}` -- 挂载后 `fileInputRef.current` 指向这个 `<input>` DOM 节点
- 调 `.click()` 跟用户用鼠标点它效果一样 -- 浏览器弹出系统文件选择框

### T2：浏览器弹出文件选择框

这一步是浏览器原生行为，不在 React 代码里。用户在文件选择框里选文件（可以选多个，因为 `multiple`），点"确定"。

### T3：onChange 触发，调 handleUpload

用户选完文件后，浏览器把 `File` 对象塞进 `input.files`（一个 `FileList`），然后触发 `onChange`：

```tsx
onChange={(e) => {
  if (e.target.files) handleUpload(e.target.files)  // ← 调 handleUpload
  e.target.value = ''                               // ← 清空 input 的值
}}
```

**为什么要 `e.target.value = ''`**：

如果用户选了 `a.txt` 上传完，再次点 Upload 又选了同一个 `a.txt`，`input.value` 没变，浏览器认为"值没变化"，**不触发 onChange**。清空后下一次选同一个文件也能正常触发。

### T4：handleUpload 并发上传所有文件

`WorkspaceFilesPanel.tsx:592-597`：

```tsx
const handleUpload = async (files: FileList, destDir: string = currentPath) => {
  await Promise.all(Array.from(files).map((f) => uploadFile(f, destDir)))
  invalidateListing()
}
```

关键点：

- `Array.from(files).map((f) => uploadFile(f, destDir))` -- 把 FileList 转成数组，**对每个文件都调一次 `uploadFile`**，返回一个 Promise 数组
- `Promise.all(...)` -- 并发执行所有上传，等全部完成
- `await` 完成后调 `invalidateListing()` -- 通知 React Query 重拉列表

### T5：uploadFile 用 XHR 跟踪进度

`WorkspaceFilesPanel.tsx:459-508` 是核心。**为什么不用 fetch**：`fetch` API **不能跟踪上传进度**（只能跟踪下载进度），要用 XMLHttpRequest 才能拿到上传进度回调。

```tsx
const uploadFile = useCallback(
  (file: File, destDir: string, relativePath?: string): Promise<void> => {
    // ① 算路径和唯一 key
    const dir = destDir.endsWith('/') ? destDir : `${destDir}/`
    const subPath = relativePath ?? file.name
    const path = `${dir}${subPath.replace(/^\/+/, '')}`
    const key = `${Date.now()}-${path}-${Math.random().toString(36).slice(2, 8)}`

    return new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest()

      // ② 上传进度回调：每次浏览器收到 TCP ACK 就触发
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          setUploads((prev) => {
            const next = new Map(prev)
            next.set(key, {
              fileName: displayName,
              loaded: e.loaded,
              total: e.total,
            })
            return next
          })
        }
      }

      // ③ 成功：从 Map 里删 key
      xhr.onload = () => {
        setUploads((prev) => {
          const next = new Map(prev)
          next.delete(key)
          return next
        })
        resolve()
      }

      // ④ 失败：也从 Map 里删 key，reject 让 Promise.all 知道
      xhr.onerror = () => {
        setUploads((prev) => {
          const next = new Map(prev)
          next.delete(key)
          return next
        })
        reject(new Error(`Upload failed: ${displayName}`))
      }

      // ⑤ 初始化进度条目（loaded: 0）
      setUploads((prev) => {
        const next = new Map(prev)
        next.set(key, { fileName: displayName, loaded: 0, total: file.size })
        return next
      })

      // ⑥ 真正发起 PUT 请求
      xhr.open('PUT', fileUrl(workspaceId, path, drive))
      xhr.send(file)
    })
  },
  [workspaceId, drive],
)
```

关键点：

- **`key` 用时间戳 + 路径 + 随机数**：防止同一秒上传两个同名文件时 key 冲突
- **`xhr.upload.onprogress`**：浏览器在上传过程中反复回调，参数 `e` 有 `loaded`（已上传字节）和 `total`（文件总字节）。每次回调都 `setUploads` 更新 Map，触发 React 重渲染
- **`new Map(prev)` + `next.set`**：不可变更新模式。不能 `prev.set(key, ...)` 直接改原 Map -- React 不会感知到变化（引用没变）。必须 new 一个新 Map，让 React 看到引用变化
- **`xhr.onload` / `xhr.onerror` 删 key**：上传完或失败，把对应条目从 Map 删掉 -- 进度条上不再显示这个文件

### T6：进度条 UI 响应 uploads 变化

`WorkspaceFilesPanel.tsx:350-361` 定义 state 和总进度：

```tsx
const [uploads, setUploads] = useState<Map<string, UploadProgress>>(new Map())
const uploading = uploads.size > 0
const overallProgress = useMemo(() => {
  if (uploads.size === 0) return 0
  let loaded = 0
  let total = 0
  for (const u of uploads.values()) {
    loaded += u.loaded
    total += u.total
  }
  return total === 0 ? 0 : Math.round((loaded / total) * 100)
}, [uploads])
```

`overallProgress` 把所有正在上传的文件的 `loaded` 加起来除以 `total` 加起来，得到整体百分比。`useMemo` 依赖 `[uploads]` -- 只要 `uploads` Map 变（每次 `setUploads` 都换新 Map），就重算。

进度条 UI `:1307-1326`：

```tsx
{uploading && (
  <div className="shrink-0 border-b border-foreground/[0.08] px-3 py-2">
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <Upload className="h-3.5 w-3.5 animate-pulse" />
      <span className="min-w-0 truncate">
        {uploads.size === 1
          ? t('...progressSingle', { name: ..., percent: overallProgress })
          : t('...progressMultiple', { count: uploads.size, percent: overallProgress })}
      </span>
    </div>
    <Progress value={overallProgress} className="mt-1.5 h-1.5" />
  </div>
)}
```

`uploading` 是 `uploads.size > 0` 的 boolean -- Map 里有条目就显示进度条，空就隐藏。`uploads.size === 1` 显示"正在上传 a.txt 45%"，`> 1` 显示"正在上传 3 个文件 45%"。

### T7：全部完成后刷新文件列表

`WorkspaceFilesPanel.tsx:219-223`：

```tsx
const invalidateListing = useCallback(() => {
  qc.invalidateQueries({ queryKey: ['workspace-files', workspaceId] })
  setAfsRefreshToken((n) => n + 1)
  setFileRefreshToken((n) => n + 1)
}, [qc, workspaceId])
```

`qc.invalidateQueries` 是 React Query 的 API -- 标记所有 `queryKey` 以 `['workspace-files', workspaceId]` 开头的查询为"过期"，下次组件渲染时会重新从服务端拉数据。**这就是为什么上传完文件后文件列表里能看到新文件**。

`setAfsRefreshToken` / `setFileRefreshToken` 是给子组件（FileViewer / AfsSharesPanel）的"刷新令牌"，子组件 useEffect 依赖这些 token，token 变了就重新拉自己的数据。

---

## 完整时序图

```
T0  用户点击 AppHeaderButton (Upload)
       fileInputRef.current?.click()
       ↓
T1  隐藏的 <input type="file"> 收到 .click() 调用
       浏览器弹出系统文件选择框
       ↓
T2  用户选了 a.txt, b.jpg, c.mp4 三个文件，点确定
       浏览器填充 input.files = FileList[3]
       触发 onChange
       ↓
T3  onChange handler:
       handleUpload(input.files)
       input.value = ''             ← 清空防重选失效
       ↓
T4  handleUpload:
       Promise.all([
         uploadFile(a.txt, '/current/path/'),
         uploadFile(b.jpg, '/current/path/'),
         uploadFile(c.mp4, '/current/path/'),
       ])
       ↓
T5  每个 uploadFile:
       setUploads(prev => new Map(prev).set(key_a, {loaded:0, total:1KB}))
       setUploads(prev => new Map(prev).set(key_b, {loaded:0, total:2MB}))
       setUploads(prev => new Map(prev).set(key_c, {loaded:0, total:10MB}))
       xhr_a.open('PUT', '/files/.../a.txt'); xhr_a.send(a.txt)
       xhr_b.open('PUT', '/files/.../b.jpg'); xhr_b.send(b.jpg)
       xhr_c.open('PUT', '/files/.../c.mp4'); xhr_c.send(c.mp4)
       ↓ uploads Map 现在有 3 个条目
       uploading = true
       进度条 UI 出现："正在上传 3 个文件 0%"
       ↓
T6  浏览器持续传数据，每收到一个 TCP ACK 就触发 xhr.upload.onprogress:
       xhr_a.upload.onprogress({loaded:500, total:1024})
         -> setUploads: key_a = {loaded:500, total:1024}
       xhr_b.upload.onprogress({loaded:1MB, total:2MB})
         -> setUploads: key_b = {loaded:1048576, total:2097152}
       ... 每秒触发多次 ...
       ↓ 每次 setUploads 都换新 Map 引用
       React 重渲染
       overallProgress 重算 (loaded_sum / total_sum * 100)
       进度条更新："正在上传 3 个文件 35%"
       ↓
T7  a.txt 传完 (1KB 很快):
       xhr_a.onload 触发
       setUploads: delete key_a
       resolve()
       ↓ uploads Map 剩 2 个条目
T8  b.jpg 传完:
       xhr_b.onload 触发
       setUploads: delete key_b
       resolve()
       ↓ uploads Map 剩 1 个条目
       进度条："正在上传 c.mp4 80%"
T9  c.mp4 传完:
       xhr_c.onload 触发
       setUploads: delete key_c
       resolve()
       ↓ uploads Map 空
       uploading = false
       进度条消失
       Promise.all 解决
       ↓
T10 handleUpload await 完成:
       invalidateListing()
       qc.invalidateQueries({queryKey: ['workspace-files', workspaceId]})
       ↓ React Query 自动重新 fetch
       文件列表更新，a.txt / b.jpg / c.mp4 出现
       setAfsRefreshToken(n+1), setFileRefreshToken(n+1)
       ↓ 子组件也刷新自己的数据
```

---

## 关键洞察

### 1. 隐藏 input + 编程 click 是前端上传的标准模式

浏览器出于安全限制，**必须用户主动操作才能打开文件选择框**。但 `<input type="file">` 默认长得丑，所以开发者隐藏它，用漂亮的按钮代替，点按钮时调 `input.click()` 偷偷触发系统文件选择框 -- 这是 React/Vue/Angular 通用模式。

### 2. `e.target.value = ''` 防止"重选同一文件不触发 onChange"

`input` 的 value 是文件路径。如果上次选了 `a.txt`，value 是 `C:\fakepath\a.txt`，再次选同一个文件浏览器认为 value 没变，不触发 onChange。每次 onChange 后清空 value，下次选任意文件都触发。

### 3. 为什么用 XHR 而不是 fetch

`fetch` API 上传时**不能跟踪上传进度**（只能跟踪响应下载进度）。要拿"已上传字节 / 总字节"，必须用 XMLHttpRequest 的 `xhr.upload.onprogress` 回调。这是 W3C 标准的历史遗留，目前 fetch 还没补上这个能力。

### 4. `Map<key, progress>` 是"多文件并发上传"的数据结构

为什么不用数组？key 是 `时间戳-路径-随机数`，每个文件一个唯一 key -- 上传完要删这个条目，Map 的 `delete(key)` O(1)。数组找文件要 O(n) 遍历。Map 也天然支持"按 key 更新某个文件进度"，不用 index。

### 5. `Promise.all` + `invalidateListing` 的协作

`Promise.all` 等所有上传 Promise 解决后才调 `invalidateListing`。如果上传失败（reject），`Promise.all` 会立即 reject，但其他上传**不会停止**（XHR 已经在浏览器里跑了，只是 Promise 状态变了）。`invalidateListing` 在 try/catch 外面，失败时不会调 -- 但已成功的文件会出现在列表里。

实际项目里更严谨的写法是 `Promise.allSettled` 等所有都结束（不管成功失败），再 invalidate。这里用 `Promise.all` 简化了。

### 6. `uploads.size > 0` 控制进度条显隐

不需要额外的 `isUploading` state -- `uploads` Map 有条目就是在上传，空就是没上传。这是"派生 state"模式：能从现有 state 推导的值，不要再开一个 state，避免不一致。

---

## 一句话总结

> 点 Upload 按钮 = 编程触发隐藏 input.click() -> 浏览器弹文件框 -> onChange 拿 FileList -> `Promise.all` 对每个文件起 XHR 上传 -> `xhr.upload.onprogress` 持续更新 `uploads` Map -> React 重渲染进度条 -> 全部完成后 `invalidateQueries` 让 React Query 重拉列表，新文件出现。
>
> 核心技巧：隐藏 input + 编程 click（绕过样式丑）、XHR onprogress（fetch 做不到上传进度）、Map by key（多文件并发跟踪）、`e.target.value = ''`（防重选失效）。
