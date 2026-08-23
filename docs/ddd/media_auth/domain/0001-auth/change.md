# 2026-08-23

用户删除语义由物理删除改为不可逆软删除。`User` 增加 `status`、`auth_version`、`disabled_at`、`deleted_at`、`deleted_by`；删除事务同时递增 `auth_version` 并写入 `UserAccessRevoked` Outbox。已删除用户及旧版本会话不得继续访问，但身份、团队关系和历史资源保留。
