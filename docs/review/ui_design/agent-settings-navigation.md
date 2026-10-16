# Agent Settings 设计与实现对齐

| 设计声明 | 实现位置 | 验证 | 状态 |
|---|---|---|---|
| Settings左侧导航统一工作空间/企业/个人管理 | pages/chat/settings-panel.tsx | 完整E2E及真实账号截图 | aligned |
| 滚动视口占满Slot右侧，阅读容器独立限宽 | settings-panel.tsx、index.css | 右边界/scrollTop/固定导航几何断言 | aligned |
| 分区导航保留Session，刷新/窄屏打开正确App | ChatPage、SlotProvider | URL/刷新/手机回归 | aligned |
| 个人与企业范围及原资源权限不变 | CatalogManager、ModelConnectionManager及原服务端API | 原资源管理回归、真实服务列表 | aligned |
| 旧链接使用相同Settings结构 | App路由、ChatPage settingsPage组合 | 原Library/Models浏览器场景 | aligned |
| L3组合，L2不跨领域导入 | domains/agent-catalog、domains/model-connection | dependency-cruiser通过 | aligned |

参考agent-platform WorkspaceSettingsPanel的固定左导航/右滚动模式；设计更新见ADR0007与agent-product-flow。完整实现review、测试和截图见 [Settings UI review](../agent-product/settings-ui/review.md)。无missing项。
