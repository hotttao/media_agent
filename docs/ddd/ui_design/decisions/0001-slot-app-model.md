# ADR-0001：采用 Slot 与 App Definition 平台模型

## 状态

Accepted

## 决策

参考 agent-platform，将 UI 功能注册为 App Definition，由 Slot Provider 管理实例，由 Slot Container 和 App Window 统一承载。Workspace 切换保留在 Shell。

当前不实现完整 plugin 系统，但内置 App 与未来 plugin 必须使用相同注册契约。

## 结果

页面布局不依赖具体业务 App；新增 App 不需要修改 Slot 状态机。代价是简单页面也必须通过注册与实例层组合。
