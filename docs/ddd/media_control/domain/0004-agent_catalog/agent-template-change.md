# Agent Template 变更记录

## 2026-08-22：Agent Template 首个垂直切片

- 新增 owner-scoped Agent Template CRUD 与不可逆归档。
- 配置以 JSONB 保存，写入前严格校验 JSON；更新/归档必须携带 `expected_version`。
- workspace-scoped Template 创建校验 Workspace 所有权；平台级 Template 使用 `workspace_id = NULL`。
