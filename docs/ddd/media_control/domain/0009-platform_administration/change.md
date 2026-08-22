# Platform Administration 变更记录

## 2026-08-22 · 版本化 Platform Setting

- 新增 Platform Setting 创建、列表、查询和 `expected_version` 乐观锁更新。
- JWT 管理员角色进入 Control Principal；普通用户由 Middleware 和 Service 双层拒绝。
- 内部管理员调用要求 Control Service Token、acting user 与 acting role 同时存在。
- `PlatformSettingChanged` 与设置事实在 PostgreSQL 同一事务写入 Outbox。
- Cluster、统计和 Admin Workspace 查询仍保留为后续只读投影切片。
