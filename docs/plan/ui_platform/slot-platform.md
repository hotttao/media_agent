# UI Platform Shell 与 Slot P0 落地计划

## 文档

- [x] 完成交互差距调研。
- [x] 更新 Shell、Slot、状态、响应式和用户旅程设计。
- [x] 新增 superseding ADR 与 P0 实现规格。

## 实现

- [x] 迁移 Workspace/Session URL 与根路由空态。
- [x] 实现 Menubar、Workspace Switcher 和 Workspace Dialog。
- [x] 升级 Profile v2 与 ProfileBackend，修复跨 Slot 单实例约束。
- [x] 实现 1/3 栏 Layout、宽度调整与 fill。
- [x] 实现 Segmented Dock、Command Slot Picker 和 Slot Empty State。
- [x] 实现 AppWindow Header Portal，并迁移内置 App header 操作。
- [x] 以 Dock 统一移动端单栏导航。

## 验证

- [x] Slot/Profile/Layout 单元测试。
- [x] Workspace/Session 深链与竞态测试。
- [x] Dock/Picker/Empty/fill E2E。
- [x] 移动端 Dock E2E。
- [x] TypeScript、Vitest、Dependency Cruiser、production build 全部通过。
