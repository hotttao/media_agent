# 变更记录

## 2026-10-11 · MCP 注册目录归 Gateway

- 将 MCP Catalog 与 MCP Definition 的权威归属从 media_plugin 移至 media_agent_gateway。
- Plugin 保留具体实现、Workspace 启用关系与外部连接，通过 Gateway 服务接口读取 Definition，不共享数据库表。
- 更新快照 Workflow，目录读取在 Plugin 本地事务外，版本冲突重新构建，失败保留上一有效快照。
- 此次为目标设计调整，接口、消息投递与发现刷新细节仍需在 spec 前定稿。

