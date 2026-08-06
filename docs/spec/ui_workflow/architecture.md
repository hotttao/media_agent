# Media Workflow UI 组合发布架构

## 1. 产品与工程边界

`ui_workflow` 是 `media_materials` 与 `media_workflow` 的唯一组合 UI 工程，包名 `@media/ui-workflow`。两个后端领域独立，前端共享发布版本、基础设施和跨域交互。

组合发布不等于合并领域：

- Materials panel 管理 Material、Generated Asset、Virtual IP、Product、Asset Kind。
- Workflow panel 管理 Template、Workflow、Execution。
- L2 domain 不得跨 domain import；跨域组合只允许发生在 L3 page 或 composition root。
- `ui_materials` 不作为正式发布工程；其已创建骨架在确认清理前保留，但不继续开发。

## 2. 四层架构

严格遵循 `ui_workflow/AGENTS.md`：

```text
L0  src/components/ui/       无领域语义的 UI primitives
L1  src/foundation/          API client、Provider、host-shim、通用 shell/store
L2  src/domains/<resource>/  单资源 types/components/hooks/lib
L3  src/pages/materials/     Materials panel 组合
    src/pages/workflow/      Workflow panel 组合
```

依赖方向为 `L3 → L2 → L1 → L0`。入口 `main.tsx`、`plugin.tsx` 与 `App.tsx` 是 composition root，不承载领域规则。

## 3. 两种发布形态

### 3.1 Standalone

- 入口：`src/main.tsx`。
- 自建 Query、Theme、Auth、Tooltip Provider。
- 使用 Desktop/Dock/Slot/AppWindow shell 组合 Materials 与 Workflow app。
- 使用 cookie session；未认证时跳转 `/api/v1/auth/login`。
- 产物目录：`dist/standalone/`。

### 3.2 Agent Platform plugin

- 入口：`src/plugin.tsx`，IIFE 格式。
- **一个插件注册两个 panel**：插件 id/发布单元为 `media-workflow`，贡献 `media_materials` 与 `media_workflow` 两个工作面。
- Plugin 是 manifest、bundle、版本、启停和回滚单元；Panel 只是插件贡献的 UI surface。不得为两个 panel 生成两份 bundle 或独立版本。
- React、ReactDOM、jsx-runtime 复用 `window.tos` 宿主单例。
- Query 与 i18n 由插件本地 Provider/实例提供；语言监听宿主 `lang.change`。
- panel 通过 host-shim 使用 Header Slot、instance state 与当前主题。
- 编译后的 Tailwind CSS 以内联字符串注入 bundle；使用稳定 style id 防重复。
- `publicDir=false`，插件交付物只有 `dist/plugin/media-workflow.js`。

## 4. 构建契约

```text
yarn build
├── yarn typecheck
├── yarn build:standalone
└── yarn build:plugin
```

两个产物必须可以独立部署和回滚，但版本号一致。插件 manifest 的 bundle URL 指向 `media-workflow.js`，并声明两个 owned panels；两个 panel 由同一 bundle 一次注册。`mcp_catalog` 可用两条条目分别控制两个 panel 在 workspace 的可见性，但不改变它们属于同一插件的事实。

## 5. 导航与状态

- Standalone 的 Desktop shell 负责打开 Materials/Workflow app。
- Plugin 不嵌套第二套 Desktop，不修改 Agent Platform URL。
- panel 内当前资源、list/show/create/edit、selected id 使用 host-shim 的 instance state，并按 panel 隔离命名空间：Materials 使用 `materials_` 前缀，Workflow 使用 `workflow_` 前缀。
- 跨卸载需要保留的状态使用 persistent state；弹窗和短期表单状态使用临时 state。
- 主操作通过 App Header Slot 注入，窗口 chrome 由宿主管理。

## 6. API 与身份边界

- 通用 HTTP、401 行为放 L1 `foundation/lib/api-client.ts`。
- 资源端点放对应 L2 domain，不允许 page 直接拼接 URL。
- Standalone dev server 可分别代理 Materials 与 Workflow 路径。
- Production/plugin 必须由部署网关提供同源路由与 cookie 身份契约；共享 cookie 并不能替代 HTTP 路由。
- 浏览器不得持有 service token，也不得调用 internal API。

## 7. 验收标准

- `yarn build` 同时生成 standalone 与单 JS plugin。
- 插件加载一次后同时注册 Materials/Workflow panel，且没有第二份 React。
- 插件无需额外 CSS 或 public assets 即可显示完整样式。
- dependency-cruiser 无 L0-L3 违规。
- Materials 与 Workflow 可跨页面组合，但 L2 domain 保持隔离。
- 两种形态共享业务组件，不复制 CRUD、DAG 或执行监控逻辑。
