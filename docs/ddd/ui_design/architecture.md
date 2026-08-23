# UI Platform 架构

## 分层

沿用 `ui_platform/contributing/architecture.md` 的四层依赖：L0 UI primitives、L1 foundation、L2 domains、L3 pages。Slot 基础设施属于 L1；Workspace、Session、Chat 业务组件属于 L2；Workspace Page 在 L3 组合它们并注册内置 App。

```mermaid
flowchart TD
  Page[Workspace Page / L3] --> Registry[Built-in App Catalog / L3]
  Page --> Shell[Desktop Shell / L1]
  Shell --> Menubar[Menubar + Workspace Switcher]
  Shell --> Layout[1/3 Column Layout]
  Shell --> Dock[Segmented Dock]
  Layout --> Slots[Slot Provider + Containers / L1]
  Registry --> Slots
  Registry --> Workspace[Workspace components / L2]
  Registry --> Sessions[Session components / L2]
  Registry --> Chat[Chat components / L2]
  Workspace --> API[API client / L1]
  Sessions --> API
  Chat --> API
```

## 运行时组合

根路由提供 Workspace 选择空态；`/w/:workspaceId` 加载 Workspace Page，`?session=` 表示可分享的活动 Session。`/admin/users` 是受保护的 platform-admin 页面，沿用 Shell 的身份与导航语义但不建立 Workspace Profile。Workspace Page 建立页面级编排上下文；App wrapper 从该上下文取数据，再把显式 props 传给 L2 组件。L2 Domain 之间不得互相导入。

Slot Provider 只理解 `AppDefinition`、`AppInstance`、Layout Slot 和 Profile，不理解 Workspace、Session 或 Chat。Dock 消费 Slot Provider；AppWindow 只渲染活动实例并提供 header portal。切换 Workspace 时由路由和 Workspace ID 重新建立 Profile 与 Session 边界。
