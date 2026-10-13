# 变更记录

## 2026-10-11 · MCP 注册目录归 Gateway

- 将 MCP Catalog 与 MCP Definition 的权威归属从 media_plugin 移至 media_agent_gateway。
- Plugin 保留具体实现、Workspace 启用关系与外部连接，通过 Gateway 服务接口读取 Definition，不共享数据库表。
- 更新快照 Workflow，目录读取在 Plugin 本地事务外，版本冲突重新构建，失败保留上一有效快照。
- 此次为目标设计调整，接口、消息投递与发现刷新细节仍需在 spec 前定稿。


## 2026-10-13 · P4 版本化目录与发布

实现当前/不可变历史/CAS、平台管理入口和内部读取、受控 host、内置 platform 来源、目录 Outbox relay。当前状态作为旧固定快照的准入屏障。目录变更提示独立于用户确认的元数据通知。
