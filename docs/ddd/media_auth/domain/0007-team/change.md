# team domain 变更记录

## 2026-09-17：Team 明确为企业内部门

- Team 从无企业归属的用户分组调整为 Organization 内的部门，新增不可变 `organization_id`。
- 一个 Organization 可以有多个 Team，一个 Team 可以创建多个 Workspace。
- 新增部门功能授权模式：默认 `inherit` 等于企业功能上限；`restricted` 时取 Organization Entitlement 与 Team Entitlement 的交集。
- 资源分享仍沿用现有 Resource Grant；本次调整不把资源 ACL 收归 Organization 或 Team Domain。
- Team 删除调整为跨服务停用：先阻止新 Workspace，再确认无非 deleted Workspace，最终保留 Team tombstone，避免历史 Workspace 和 Resource Grant 成为悬挂引用。
