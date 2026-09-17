# workspace domain 变更记录

## 2026-09-17：Workspace 归属企业部门

- Organization 定义企业客户与功能授权上限，Team 定义企业内部门，Team 与 Workspace 为一对多。
- Workspace 从用户 owner 模型调整为 Team 组织所有权，新增不可变 `organization_id`、`team_id`；`created_by` 仅保留创建审计。
- Workspace action 同时受 Organization 状态、Team Membership、Team 有效功能和 Workspace 状态约束。
- Workspace 仍是业务资源与运行数据的隔离边界；资源分享继续使用资源服务自己的 Resource Grant。
- Workspace.Create 必须拒绝 decommissioning/decommissioned Team，并参与跨服务 Team Decommission Workflow 的引用检查。
