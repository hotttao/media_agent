# ui_workflow 前端实现方案

> 为 media_workflow / media_materials 两个后端微服务开发组合前端。`ui_workflow` 是唯一 canonical UI 工程，以同一版本同时发布 standalone 与 `agent-platform/web` plugin。两种模式都要符合 agent-platform/web 的样式。
>
> 本文是调研结论 + 实现设计的存档，作为后续编码依据。

## 0. 决策摘要（已确认）

| 决策 | 结论 |
|---|---|
| 工程位置 | 合并到 `ui_workflow` 单工程；`ui_materials` 不作为正式发布工程 |
| plugin 嵌入粒度 | 拆成多个 panel tab（按资源大类聚合，非每实体一 panel） |
| plugin 发布粒度 | 一个 `media-workflow` 插件 bundle，贡献 Materials 与 Workflow 两个 panel |
| 首期范围 | 先 materials（CRUD 为主），再 workflow（DAG 编辑器 + SSE 执行监控） |
| 目录规范 | 严格遵循 `ui_workflow/contributing/architecture.md` 的 L0-L3 分层与 foundation 6 子目录 |
| 认证 | materials 与 workflow 共享 `JWT_SECRET` + 同 cookie 域 `mw_token`；standalone 走 OAuth via media_auth，plugin 复用宿主态 |
| 复用单元 | app 组件（非 pages/router），内部用 instance state 切视图 |

## 1. 调研结论

### 1.1 仓库现状

仓库顶层三个同构前端工程（Vite + React 18 + Tailwind + shadcn/ui + L0-L3 + biome/dependency-cruiser/knip/i18n-tracker/e2e）：

- `ui_platform`：media_auth 前端，已有 login/register/oauth/dashboard + foundation 基建（api-client/auth-provider/protected-route），`src/domains/` 为空，无菜单/布局，i18n 未初始化。
- `ui_workflow`：为 media_workflow 准备的**空骨架**（与 ui_platform 同构），业务代码为零。**本工程为开发目标**。
- `ui_example`：另一空骨架。

`ui_workflow/contributing/architecture.md` 与 `ui_platform` **完全一致**（diff 无输出），规范统一。

### 1.2 agent-platform 的 plugin 模型（核心）

来源：`docs/wiki/web/learn/01-host-and-plugins.md`、`05-slots-and-app-registry.md`、`agent-platform/web/src/hooks/useWsApps.ts`、`components/shell/windows/AppWindow.tsx`、`components/workspace/WorkspaceFilesPanel.tsx`。

**plugin = 注册一个或多个 app（panel），不是塞一个 SPA。** 四要素：

1. **注册**：IIFE bundle 加载后调 `window.tos.registerPanel({ id, label: ()=>string, component })`（`lib/panel-registry.ts:8`）。`id` 必须匹配 `mcp_catalog.ui_panel` 字段（服务端控制可见性）。
2. **挂载**：panel.component 被 `getPluginApp` 包成 `AppDefinition.Component`（`wsApps.tsx:425-461`），挂到 workspace 某个 slot，由宿主 `AppWindow` 渲染。组件只接收 `{ workspaceId, instanceId }`。
3. **无路由**：app 内部 list↔detail↔edit 用 `useInstancePersistentState` / `useInstanceState`（per-instance，key 三元组 `workspaceId,instanceId,key`）切换 `viewingPath`/`selectedId`，**不用 react-router**（见 `WorkspaceFilesPanel.tsx` 的 `viewingPath` + `navigate`）。
4. **header 注入**：宿主 `AppWindow` 提供 `AppHeaderSlotContext`（一个 DOM 节点，`AppWindow.tsx:48,122`），app 用 `useAppHeaderSlot() + createPortal` 把按钮/面包屑注入。窗口 chrome（边框、关闭/最大化）归宿主。

**宿主 Provider 树已就位**（`main.tsx:34-49`）：`QueryClientProvider > ThemeProvider > BrowserRouter > AuthProvider > TooltipProvider > App`。app 直接 `useAuth`/`useQuery`/`useTranslation`/`useAppHeaderSlot`/`useInstancePersistentState`，**plugin 不自挂 AuthProvider/QueryProvider/I18nProvider**。

**`window.tos` 暴露全表**（`lib/host.ts:41-73`）：
- 顶层：`version` `React` `ReactDOM` `ReactJSXRuntime` `events` + 注册三件套 `registerPlugin/registerPanel/registerToolRenderer`
- `ui`：`Markdown` `useResolvedTheme`
- `workspace`：`useSlotContext` `useAgentSessionActions` `useAgentSessionStore` `useSessionNavigation` `useInstanceState` `useInstancePersistentState`
- `windows`：`useAppHeaderSlot` `AppHeaderButton`
- `tools`：`getMcpText` `unwrapMcpInput` `safeParseResult` `truncate` `jsonPreview`

> ⚠️ **未暴露** `useQuery`/`useMutation`/`useTranslation`/`useAuth`。这是 plugin 复用 app 代码的关键 gap（见 §4.3）。

### 1.3 后端 API 与领域模型

来源：`idl/media_materials/*.proto`、`idl/media_workflow/*.proto`、`media_*/biz/router/*/*.go`、Traefik 配置。无 swagger/openapi spec。

**media_materials**（base `/api/v1/{materials,products,virtual-ips,asset-kinds,generated-assets}`，标准 CRUD 为主）：

| 实体 | 关键端点 | 复杂度 |
|---|---|---|
| AssetKind | Register/Get/List/Update/Delete + BatchGet(内部) | 低 |
| Material | CRUD + ListByOwner + AddBinding/RemoveBinding | 低 |
| GeneratedAsset | Get/List/ListBySource/Delete + Upsert(内部，workflow 调) | 低（只读） |
| VirtualIp | CRUD + Archive + Grant/Revoke + ChangeVisibility | 中（含授权） |
| Product | CRUD + Archive | 低 |

**media_workflow**（base `/api/v1/{workflows,templates,jobs,step-runs}`）：

| 实体 | 关键端点 | 复杂度 |
|---|---|---|
| Template | CRUD + Archive/Publish/Grant/Revoke/ChangeVisibility + GetVersion | 中（prompt 编辑器 + 动态 slot） |
| Workflow | CRUD + Validate/Archive/Publish/Grant/Revoke/ChangeVisibility | **高（DAG 可视化编辑器）** |
| Execution(Job+StepRun) | SubmitJob/Get/List/Cancel/Retry + ConfirmStepRun/RegenerateStepRun + Claim/Complete/Fail | **高（SSE 实时监控 `/api/v1/jobs/:id/events` + 候选确认）** |

**跨服务聚合**（后端 RPC 对前端不可见，前端只发 id）：提交 Job 时从 materials 选素材组装 bundle；查看产物时拿 `output_asset_id` 反查 materials。参考 `docs/adr/0006-reachability-layering.md`。

### 1.4 认证链路（关键修正）

来源：`media_workflow/biz/shared/auth/middleware.go`、`docs/media_auth/domain/0005-oauth/oauth.md`、ADR 0001/0002。

1. **前端不能拿 media_auth 的 OAuth access_token 直接调 materials/workflow**：后端 Bearer 路径验证的是 service token（`tos_xxx`，查表），OAuth access_token 是 JWT，走 Bearer 会被当 service token 查表 -> 401。
2. **正确路径是 cookie**：前端经 OAuth 登录后，**materials/workflow 后端给前端发自己的 session cookie `mw_token`（JWT，本地 HS256 验签）**，后续请求靠 cookie。
3. **materials/workflow 的 auth middleware 已内建 OAuth 登录流程**（`middleware.go:402-463`）：
   - `GET /api/v1/auth/login` -> 302 跳 media_auth `/api/oauth/authorize`（带 PKCE S256）
   - `GET /api/v1/auth/callback?code=...` -> 后端换 token + 调 userinfo + 签 `mw_token` cookie + 302 回首页
   - `GET /api/v1/auth/me` -> 读 cookie 返回当前用户
   - `POST /api/v1/auth/logout` -> 清 cookie

**对设计的影响**：standalone 前端不碰 OAuth code/token，只需"跳转 `/api/v1/auth/login` + 享受 cookie"。两种模式 api-client **完全相同**（`credentials:'include'` + cookie），区别只在"如何获得 cookie + 401 处理"。

**前置依赖**：materials 与 workflow 共享 `JWT_SECRET` + 同 cookie 域 `mw_token`（已确认），使一次 OAuth 登录两服务都认。

## 2. 工程结构（严格 L0-L3 + 6 子目录）

```
ui_workflow/src/
├── main.tsx                    # 产物① standalone 入口：自建 shell（拷 agent-platform main.tsx Provider 栈）
├── App.tsx                     # standalone 根路由（仅 /login + / 工作区）
├── plugin.tsx                  # 产物② plugin 入口：registerPanel 注册各 app
├── index.css                   # theme CSS 变量（拷 agent-platform internal/theme/variables.css）
├── components/ui/              # L0 shadcn primitives
├── foundation/                 # L1 - 严格 6 子目录（见 §2.1）
├── domains/                    # L2
│   ├── materials/              # types.ts / lib/api.ts / hooks/ / components/
│   └── workflow/               # （下期）
├── pages/                      # L3
│   ├── materials/              # app.tsx（app 组合器）+ list/create/edit/show.tsx
│   └── workflow/               # （下期）
└── locales/                    # i18n 资源
```

### 2.1 foundation/ 6 子目录（符合 architecture.md）

```
foundation/
├── components/   # protected-route, app-window(拷), desktop(拷), dock(拷), slot-container(拷), data-table, resource-form
├── hooks/        # use-deletion-confirm, use-column-visibility 等
├── lib/          # utils, api-client, i18n(工厂), constants, plugin-host(window.tos 类型+registerPanel), instance-state(standalone 拷)
│   └── host-shim/  # index(构建期选) + standalone.ts + plugin.ts   ← 关键，见 §4.2
├── providers/    # auth-provider(OAuth), theme-provider, query-provider, i18n-provider, slot-provider
├── stores/       # instance-state-store(拷), workspace-profile-store(拷) - standalone 用
└── types/        # auth, plugin-host(window.tos/NapHost), shared serving types
```

**API 分层**（关键）：通用 HTTP 封装放 `foundation/lib/api-client.ts`（cross-cutting）；资源专属端点放 `domains/<name>/lib/api.ts`（resource-specific，符合 L2 lib 放 "resource-specific utilities"）。

### 2.2 入口在 src 根（不引入 app/ 子目录）

`main.tsx`（standalone）+ `plugin.tsx`（plugin）+ `App.tsx` 平级在 src 根，是双产物需求的最小必要新增，符合 ui_platform 入口在 src 根的约定。

## 3. 模式区分设计

### 3.1 核心原则：复用单元 = app 组件，模式区分只在「外壳」

```
                app 组件 (MaterialsApp / WorkflowApp / ...)
                ├─ 通过 host-shim 访问能力（useAppHeaderSlot / useInstancePersistentState / ...）
                ├─ 内部用 instance state 切 list↔detail↔edit，header 用 createPortal
                └─ 模式无关，100% 复用
                        │
        ┌───────────────┴───────────────┐
   standalone shell                     plugin（宿主 shell）
   拷贝 agent-platform 的                registerPanel 注册 app
   Desktop/Dock/Slot/AppWindow/         宿主提供 AppWindow + Provider 树
   instance-state-store/Provider 栈       app 跑在宿主 Desktop 的 slot 里
   自建一个能跑 app 的环境
```

**standalone 不是"另写一套 SPA 路由"，而是拷贝 agent-platform 的 shell 让 app 跑在同样环境**，从而 app 代码 + 样式两模式一致。顶层路由仅 `/login`（OAuth 回调落点）和 `/`（工作区），工作区内全走 slot+app，无子路由。

### 3.2 两种模式对照

| 维度 | standalone | plugin |
|---|---|---|
| 外壳 | 自建 shell（拷 Desktop/Dock/Slot/AppWindow + Provider 栈） | 宿主提供 |
| 路由 | 顶层 BrowserRouter（/login + /），工作区内 app 用 instance state | 无路由，panel=app 组件，instance state |
| 认证 | 自建 AuthProvider：跳 `/api/v1/auth/login` 走 OAuth + 401 回跳 | 宿主 AuthContext 已提供 user；cookie 同域携带 |
| theme | 自建 ThemeProvider + index.css 变量 | 宿主 CSS 变量级联，app 用语义类名 |
| i18n | 自建 i18next（拷 lib/i18n.ts） | 宿主 i18next（见 §4.3 gap） |
| query | 自建 QueryClientProvider（拷 main.tsx） | 宿主 QueryClient（见 §4.3 gap） |
| header slot | 自建 AppHeaderSlotContext（拷 AppWindow） | `tos.windows.useAppHeaderSlot` |
| instance state | 自建 instance-state-store（localStorage） | `tos.workspace.useInstancePersistentState` |
| API 凭证 | cookie `mw_token`（OAuth 后由后端签发） | cookie `mw_token`（宿主已注入或同域） |

### 3.3 关键纠正（相对早期错误设计）

- ❌ MemoryRouter：plugin 不用路由，用 instance state 切视图
- ❌ PluginAuthBridge：多余，宿主 AuthContext 已提供 user
- ❌ plugin 内自挂 QueryProvider/I18nProvider：多余，宿主已挂（standalone 才自挂）
- ❌ panel 内套 routes.tsx：plugin 是单个自包含 app 组件，不是路由树
- ✅ app 组合器放 `pages/<resource>/app.tsx`，组合 list/create/edit/show 视图 + 通过 shim 切状态

## 4. 关键技术设计

### 4.1 app 组件骨架（模式无关）

```tsx
// pages/materials/app.tsx -- standalone Desktop 和 plugin registerPanel 共用
import { useAppHeaderSlot, useInstancePersistentState, useInstanceState } from '@/foundation/lib/host-shim'
import { AppHeaderButton } from '@/foundation/lib/host-shim'  // shim 转发 tos.windows.AppHeaderButton 或本地
import { MaterialsList, MaterialsShow, MaterialsCreate } from './list' /* ... */

export function MaterialsApp({ workspaceId, instanceId }: { workspaceId: string; instanceId: string }) {
  const headerSlot = useAppHeaderSlot()
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(instanceId, 'selectedId', () => null)
  const [view, setView] = useInstanceState<'list' | 'show' | 'create'>(instanceId, 'view', () => 'list')

  return (
    <>
      {headerSlot && createPortal(
        <AppHeaderButton icon={Plus} onClick={() => setView('create')} />,
        headerSlot,
      )}
      {view === 'list' && <MaterialsList onSelect={(id) => { setSelectedId(id); setView('show') }} />}
      {view === 'show' && selectedId && <MaterialsShow id={selectedId} onBack={() => setView('list')} />}
      {view === 'create' && <MaterialsCreate onDone={() => setView('list')} />}
    </>
  )
}
```

### 4.2 host-shim：app 访问能力的统一层

app 不直接 import 宿主模块（plugin bundle 里没有），调 ui_workflow 的 `host-shim`。两实现，构建期按 `VITE_TARGET` 切换（DCE 掉不用的一支）：

```ts
// foundation/lib/host-shim/index.ts
import * as standalone from './standalone'
import * as plugin from './plugin'
const impl = import.meta.env.VITE_TARGET === 'plugin' ? plugin : standalone
export const useAppHeaderSlot = impl.useAppHeaderSlot
export const useInstancePersistentState = impl.useInstancePersistentState
export const useInstanceState = impl.useInstanceState
export const AppHeaderButton = impl.AppHeaderButton
```

| 能力 | standalone 实现 | plugin 实现 |
|---|---|---|
| `useAppHeaderSlot` | 自建 `AppHeaderSlotContext`（拷 AppWindow） | `window.tos.windows.useAppHeaderSlot()` |
| `useInstancePersistentState` | 自建 `instance-state-store`（localStorage） | `window.tos.workspace.useInstancePersistentState` |
| `useInstanceState` | 自建（内存） | `window.tos.workspace.useInstanceState` |
| `AppHeaderButton` | 本地组件 | `window.tos.windows.AppHeaderButton` |
| `useCurrentUser` | 自建 AuthProvider（OAuth） | 调 `GET /api/v1/auth/me`（cookie）或不依赖 |
| `useQuery`/`useMutation` | react-query 自带，QueryClientProvider 挂 main.tsx 外层 | react-query 自带，QueryClientProvider 挂 panel 内（§4.3 方案 C） |
| `useTranslation` | react-i18next 自建实例 | react-i18next 自建实例 + 监听 `tos.events`（§4.3 方案 C） |

### 4.3 ⚠️ plugin 模式访问 react-query / i18n（gap，待确认）

**机制：为什么 inject 了还拿不到**（`plugin-loader.ts:88-97` `injectScript`）

plugin 用普通 `<script src>` 注入，代码确实跑在宿主页面全局，能访问 `window`/DOM/`window.tos`。但 **inject 共享的是全局作用域，不是 bundle 内 import 的第三方库实例**。拿不到 react-query 的根因是 React Context 的匹配机制：

1. `useQuery` 内部 `useContext(QueryClientContext)`，`QueryClientContext` 是 `@tanstack/react-query` 模块顶层 `React.createContext()` 的返回对象，**按对象身份（identity）匹配**。
2. 两份 react-query = 两次 `createContext()` = 两个不同对象 X / Y。即使源码一字不差，对象引用不同。
3. 宿主 `<QueryClientProvider client={qc}>`（`main.tsx:36`）用宿主那份（Y）注入 qc；plugin `useQuery` 用 plugin 副本那份（X）查 -> X 查不到 Y 贴的 -> "No QueryClient set"。
4. `installHost`（`host.ts`）只挂 `React/ReactDOM/ReactJSXRuntime`（hooks 要求全局单例，强制共享），**明确不挂 react-query/i18next**（`01-host-and-plugins.md` 抓手1："展示层库 shadcn/Radix/lucide/zustand/i18next 和领域 API 不进主机，插件自己解决"）。React 必须共享（否则 hooks 崩），react-query 没这硬约束（两份能各自跑，只是 context 不通），所以 host 不共享。
5. 旁证：`how-to-custom-ui.md` §3 的 plugin 范例只用 `fetch + useState`，不用 react-query。agent-platform 现有 plugin 实践没暴露过这个 gap。ui_workflow 要 app 代码两模式复用（standalone 用了 react-query）才第一次撞上。

**事实**：
- 宿主 `main.tsx:19,36` 挂了 `QueryClientProvider`，`lib/i18n.ts` 模块级初始化 i18next 单例。
- 但 `vite.config.ts:106` 的 `dedupe` 只对 web+ui-sdk（内部 monorepo）生效，**外部 plugin bundle 自带的 react-query/react-i18next 副本和宿主不是同一份**，其 `useQuery`/`useTranslation` 读不到宿主 Provider 注入的实例。
- `window.tos` **未暴露** useQuery/useTranslation（`host.ts:41-73`）。

**standalone 模式**：照搬 `main.tsx` Provider 栈 + `lib/i18n.ts` 初始化，无问题。

**已定方案 C**：plugin 自带 react-query/react-i18next 副本 + panel 内自挂 Provider。无需 agent-platform 配合，接受 cache 隔离 + bundle 增大的代价。

实现要点：
- **构建**：plugin bundle 把 `@tanstack/react-query`、`i18next`、`react-i18next` 打进 bundle（与 react-router 等一起自带）。`external` 只保留 `react`/`react-dom`（hooks 强制单例）。
- **QueryClient**：`foundation/lib/query.ts` 导出 module-level 单例 `queryClient`。standalone 在 `main.tsx` 外层挂 `<QueryClientProvider client={queryClient}>`；plugin 在每个 panel 组件内挂同一 `<QueryClientProvider>`。同 plugin 多 panel 共享缓存，与宿主缓存隔离（可接受）。
- **i18n**：`foundation/lib/i18n.ts` 导出 `createI18n()` 工厂 + module-level 单例。standalone 用 LanguageDetector（localStorage `tos-language` + navigator）；plugin 额外监听 `window.tos.events.on('lang.change', lng => i18n.changeLanguage(lng))` 跟随宿主切换。I18nProvider 内部检测 `window.tos` 决定是否订阅，一个 Provider 适配两模式。
- **app 代码**：直接 `import { useQuery } from '@tanstack/react-query'`、`import { useTranslation } from 'react-i18next'`，不经 shim。两模式都读各自 Provider 注入的实例，**统一**。

**对 shim 的简化**（修订 §4.2）：`useQuery`/`useMutation`/`useTranslation` **不需要两实现**，app 直接用库 hook。shim 只对 **nap 专属能力**（`useAppHeaderSlot`/`useInstancePersistentState`/`useInstanceState`/`AppHeaderButton`）做两实现（plugin 转发 tos，standalone 自建）。

**已知约束**（接受）：plugin 的 query 缓存与宿主隔离（宿主 `invalidateQueries` 不波及 plugin，反之亦然）；plugin bundle 体积较大（含 react-query/react-i18next/react-router 副本）。

### 4.4 认证设计

**api-client（两模式共享）**：
```ts
// foundation/lib/api-client.ts
type UnauthorizedHandler = () => void
let onUnauthorized: UnauthorizedHandler = () => {}
export function configureApi(opts: { onUnauthorized?: UnauthorizedHandler }) { /* ... */ }

class ApiClient {
  private baseUrl = '/api/v1'
  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      ...init, headers: { 'Content-Type': 'application/json', ...init?.headers },
      credentials: 'include',  // 两模式都靠 cookie
    })
    if (res.status === 401) { onUnauthorized(); throw new ApiClientError('Unauthorized', 401, {}) }
    if (!res.ok) { /* 抛 ApiClientError */ }
    return res.json()
  }
  getMe = () => this.request<User>('/auth/me')
  logout = () => this.request('/auth/logout', { method: 'POST' })
}
export const api = new ApiClient()
```

**standalone** 入口注入：`configureApi({ onUnauthorized: () => location.href = '/api/v1/auth/login' })`
**plugin**：不注入（401 上抛，宿主处理）。

**StandaloneAuthProvider**（拷 ui_platform auth-provider 改造，登录改 OAuth 跳转）：
- 挂载调 `api.getMe()` 恢复（`GET /api/v1/auth/me`）
- `login()` = `window.location.href = '/api/v1/auth/login'`（后端 302 走 OAuth）
- `logout()` = POST `/api/v1/auth/logout`
- 401 -> 跳 `/api/v1/auth/login`

### 4.5 构建配置（vite 双 target）

```ts
// vite.config.ts（要点）
const target = process.env.VITE_TARGET ?? 'standalone'
if (target === 'plugin') {
  return {
    build: {
      outDir: 'dist/plugin',
      lib: { entry: 'src/plugin.tsx', formats: ['iife'], name: 'MediaWorkflowPlugin', fileName: () => 'media-workflow.js' },
      rollupOptions: {
        external: ['react', 'react-dom', 'react-dom/client'],
        // 若 §4.3 方案 A/B 落地，追加 external '@tanstack/react-query','i18next','react-i18next' + globals
        output: { globals: { react: 'window.tos.React', 'react-dom': 'window.tos.ReactDOM' } },
      },
    },
  }
}
// standalone：outDir=dist/standalone（index.html -> main.tsx）+ dev proxy 分发 materials/workflow
```

> **external 只 react/react-dom**：agent-platform 是 react-router v6，ui_workflow 是 v7，版本不兼容不能共享，router 等打进 plugin bundle 自带，只 React 必须用宿主单例（rules of hooks）。
>
> **单 JS 交付**：plugin 入口以 `?inline` 导入 `index.css` 并注入稳定 style id；plugin config 设置 `publicDir=false`，最终只交付 `dist/plugin/media-workflow.js`。

## 5. Panel 拆分清单（按后端领域聚合）

每个 panel `id` 需后端在 `mcp_catalog.ui_panel` 登记。

同一 plugin bundle 注册两个 panel：

- `media_materials`：Material、Generated Asset、Virtual IP、Product、Asset Kind。
- `media_workflow`：Template、Workflow、Execution。

panel 内按资源切换，panel 间按后端领域隔离。对应的 `mcp_catalog.ui_panel` 必须分别登记这两个稳定 id。

## 6. 首期路线图（materials 先行）

1. **基建**：拷贝 ui_platform foundation → 扩展 api-client 多服务（cookie + onUnauthorized）；落 theme/i18n/auth；拷贝 agent-platform shell（Desktop/Dock/Slot/AppWindow/instance-state-store）+ Provider 栈（参考 `main.tsx`/`lib/i18n.ts`）；建 host-shim（standalone + plugin 两实现）。
2. **materials 域**：5 实体 types/lib/api/hooks/components + pages（app 组合器 + list/create/edit/show）。
3. **standalone 跑通**：BrowserRouter + Desktop 菜单 + 全部 materials app 可访问，接 Traefik 网关联调 + OAuth 登录。
4. **plugin 跑通**：3 个 materials panel registerPanel，`window.tos` 嵌入验证（含 panel id 匹配 mcp_catalog）。**前置：确认 §4.3 plugin query/i18n 方案**。
5. **验收**：双产物都能 CRUD materials，样式与 agent-platform 一致。

workflow 域（DAG 编辑器、SSE 监控）作为下一阶段，复用首期打通的双产物链路。

## 7. 待确认项 / 前置依赖

| # | 项 | 影响 | 负责方 |
|---|---|---|---|
| 1 | **plugin 模式 react-query/i18n**（§4.3） | 已定方案 C：plugin 自带副本 + panel 内自挂 Provider | 无需宿主配合；已知约束：cache 隔离 + bundle 增大 |
| 2 | **panel id ↔ mcp_catalog** | panel tab 可见性 | 后端在 `mcp_catalog.ui_panel` 登记 3 个 materials panel id |
| 3 | plugin 模式凭证打通 | plugin 调 materials 是否带得上 cookie | 确认 agent-platform 与 materials/workflow 同域或代理打通 `mw_token` |
| 4 | standalone 登录入口 | OAuth 走 materials 还是 workflow 的 `/api/v1/auth/login` | 两服务共享 JWT_SECRET+cookie 域（已确认），任一即可，建议固定走 materials |
| 5 | react-router 版本差 | plugin bundle 自带 v7，不与宿主 v6 共享 | 已用 external 策略规避（打进 bundle） |

## 8. 可拷贝代码清单

**从 `ui_platform/` 拷**（同构工程，标注 `Adapted from`）：
- `foundation/lib/api-client.ts`（扩展为 cookie + onUnauthorized 注入）
- `foundation/providers/auth-provider.tsx`（改造为 OAuth 跳转）
- `foundation/components/protected-route.tsx`
- `foundation/lib/utils.ts`、`foundation/types/auth.ts`
- `components/ui/*`、`tailwind.config.js`、`biome.json`、`.dependency-cruiser.cjs`、`tools/i18n-tracker.cjs`、e2e 框架（含 EventSource mock，workflow SSE 会用到）

**从 `agent-platform/web/` + `internal/` 拷**：
- `web/src/main.tsx` 的 Provider 栈结构（standalone 入口照搬）
- `web/src/lib/i18n.ts` 的 i18next 初始化
- `web/src/lib/host.ts:41-73` 的 `NapHost` 接口（作为 `window.tos` 类型）
- `web/src/components/shell/` 的 Desktop/Dock/SlotContainer/AppWindow（standalone shell 复刻）
- `web/src/contexts/SlotContext.tsx` + `stores/instance-state-store.ts` + `stores/workspace-profile-store.ts`（standalone instance state）
- `internal/theme/src/variables.css`（CSS 变量）
- `web/tailwind.config.js` 的 oklch 映射段

**plugin 模板**：`docs/wiki/plugin/how-to-custom-ui.md` §3 的 IIFE 结构 + `build.lib`/`external`/`globals` 配置。

**standalone shell 搭建参考**：`docs/wiki/web/learn/ui.md`（页面骨架与从零搭建 8 步）、`00-overview.md`（Provider 嵌套树）。
