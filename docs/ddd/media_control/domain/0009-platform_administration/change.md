# Platform Administration 变更记录

## 2026-08-22 · 版本化 Platform Setting

- 新增 Platform Setting 创建、列表、查询和 `expected_version` 乐观锁更新。
- JWT 管理员角色进入 Control Principal；普通用户由 Middleware 和 Service 双层拒绝。
- 内部管理员调用要求 Control Service Token、acting user 与 acting role 同时存在。
- `PlatformSettingChanged` 与设置事实在 PostgreSQL 同一事务写入 Outbox。
- Cluster、统计和 Admin Workspace 查询仍保留为后续只读投影切片。

## 2026-08-22 · Cluster 状态投影

- 新增 `online/degraded/offline/unknown` Cluster 状态和 JSON capacity 读模型。
- 仅有效 Control Service Token 建立的内部 `system` Principal 可以上报；Admin/System 角色可以查询。
- Service 与 PostgreSQL conditional upsert 双层保证 `observed_at` 单调，迟到和重复观察不覆盖新状态。
- 增加普通用户、非 system 上报拒绝以及迟到/较新观察的单测和真实 HTTP E2E。
