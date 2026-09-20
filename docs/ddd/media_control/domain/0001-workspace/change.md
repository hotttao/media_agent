# workspace domain 变更记录

## 2026-09-17：Workspace 归属企业部门

- Organization 定义企业客户与功能授权上限，Team 定义企业内部门，Team 与 Workspace 为一对多。
- Workspace 从用户 owner 模型调整为 Team 组织所有权，新增不可变 `organization_id`、`team_id`；`created_by` 仅保留创建审计。
- Workspace action 同时受 Organization 状态、Team Membership、Team 有效功能和 Workspace 状态约束。
- Workspace 仍是业务资源与运行数据的隔离边界；资源分享继续使用资源服务自己的 Resource Grant。
- Workspace.Create 必须拒绝 decommissioning/decommissioned Team，并参与跨服务 Team Decommission Workflow 的引用检查。

## 2026-09-20：固定 Workspace 授权接口边界

- 创建请求显式携带 Organization/Team，`created_by` 只从认证主体取得。
- 普通列表按调用者可访问的 Team 收敛；详情和更新从持久化 Workspace 归属反查权限。
- 增加受内部凭证保护的 Workspace access check，资源服务不得信任客户端自报 Workspace/Team。

### 能力覆盖矩阵

| 原能力 | 目标限界上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| 个人 owner 创建/列出/修改 Workspace | media_control | workspace | Workspace | workspace_lifecycle | deprecated-with-reason：公开 Workspace API 已切 Team 授权；owner 字段仅供历史数据迁移 |
| Team 下创建 Workspace | media_control | workspace | Workspace | workspace_lifecycle | covered |
| 按 Team/Organization 查询 Workspace | media_control | workspace | Workspace Read Model | 无：纯查询 | covered |
| 资源服务检查 Workspace action | media_control | workspace | Workspace Access Decision | authorization_projection（上游 Team 判定） | covered |
| Team 停用前检查活动 Workspace | media_control | workspace | Workspace Reference Query | team_decommission | missing |
