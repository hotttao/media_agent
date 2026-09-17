# ADR-0005：通过 Shell Context 和 Navigation Port 注入宿主导航

## 状态

Accepted。

## 背景

Materials 与 Workflow 需要先以独立应用交付，之后作为 App 放入 Agent Platform Slot。若业务组件直接渲染顶部导航、依赖本地 Router 或调用 `window.tos`，迁移时会产生两套组件和状态所有者。

## 决策

L1 定义稳定 Shell Context，暴露当前 Organization、Team、Workspace、Principal、capabilities 和 Navigation Port。standalone adapter 用本地 Router/Menubar 实现，plugin adapter 用宿主 API 实现。L2 App 只消费 Context 和 route intent；全局 Menubar 永远由宿主拥有，App 局部动作继续使用 App Header Portal。

## 结果

Materials/Workflow 的 L2/L3 组合可以跨宿主复用，层级选择状态只有一个所有者。代价是所有导航目的需要定义稳定 route intent，且两种 adapter 都必须通过契约测试。
