# 共享资源访问语言

统一媒体资源在不同服务中的 Scope 分类和 ACL 语义。它是共享语言，不是独立服务；每个资源所属服务保存 Grant 并执行最终授权，`media_auth` 只提供 Team Membership 事实。

## 业务术语词典

**Scope（业务范围）**:
资源所属业务线的不可变分类，例如 `ai_clothing` 或 `ai_guoxue`；同一业务流程禁止跨 Scope 引用。
_禁用替代词_：Workspace（Workspace 是租户边界）、Tenant、Business

**Resource Visibility（资源可见性）**:
资源的可见度档位：`private | team | public`；public 只放开读取，不绕过 Workspace 和 Scope 约束。
_禁用替代词_：ACL、Permission

**Resource Grant（资源授权）**:
资源所属服务保存的 `(resource_id, team_id) -> viewer | editor` 显式授权；Team Membership 由 `media_auth` 提供。
_禁用替代词_：Team Membership、Share Link

**My Permission（有效资源权限）**:
资源服务根据 owner、Resource Grant、Team Membership 和 Visibility 推导的 `owner | editor | viewer | public | none`。
_禁用替代词_：Role、Claim、Principal

**Resource Owner（资源所有者）**:
创建并管理资源 ACL 的用户；owner 权限不需要额外 Resource Grant。
_禁用替代词_：Workspace Owner、Team Admin
