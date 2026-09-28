---
name: team
description: 企业内部门、部门成员、部门功能范围与邀请管理
dependencies:
    - auth
    - organization
related_workflow:
    - team_invite_accept
    - team_decommission
---

## 一句话职责
Team Domain 维护 Organization 内部门、部门成员关系、部门有效功能和邀请链接；同时为 [Resource Grant](../../CONTEXT.md#resource-grant资源共享授权) 提供 `team_id` 维度。

## 边界

### 做什么
- Team 自身 CRUD：在一个 active Organization 内创建部门、查询、更新和删除；Organization 归属创建后不可变。
- 成员管理：列出 team 成员、加入（admin 主动 add 或邀请接受）、降级/升级 role、移除（含 self-remove）。
- 部门功能：默认继承 Organization 的全部 Entitlement；企业管理员可切换为 restricted 并授予企业功能的子集。
- 邀请链接：admin 签发带过期时间的 `tinv_` 随机 token；admin 撤销；列出未过期邀请。
- 内部只读查询：按 user 列出可见 team、按 token 查邀请（供预览/接受）；供 [auth](../0001-auth/auth.md) 等 domain 用 `getMembership(teamID, userID)` 做权限判定。

### 不做什么
- 不做资源级 ACL 业务（`visibility='team'`、Resource Grant 校验）——team 仅暴露 team_id 与成员关系，下游服务（media_materials / media_workflow）按需取用。
- 不签发 OAuth Client / 不接管 Session——分别归 [oauth](../0005-oauth/oauth.md) / [auth](../0001-auth/auth.md)。
- 不管理 API Token、identity 绑定、企微登录通道——分别归 [api_token](../0004-api_token/api_token.md) / [identity](../0002-identity/identity.md) / [wecom](../0003-wecom/wecom.md)。
- 不实现跨 workspace 的 teamwork 协作（多 workspace + AFS 共享）——那是 control-plane 现存 `teamwork` 域，与本 team domain 不同概念；本服务不迁 teamwork。
- 不拥有企业信息和企业功能上限——归 [organization](../0009-organization/organization.md) Domain。
- 不拥有 Workspace；一个 Team 可创建多个 Workspace，但 Workspace 事实归 `media_control.workspace`。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建 Team | 企业管理员在指定 Organization 内创建部门 | `CreateTeam` `POST /api/teams`（请求必须含 `organization_id`） |
| 列出我所在的 team | 按当前 user 查所有 membership，含 my_role / member_count | `ListMyTeams` `GET /api/teams` |
| 查看 team 详情 | Team 成员或 Organization admin 可读 | `GetTeam` `GET /api/teams/{id}` |
| 修改 team | Organization admin 或 Team admin 修改 name/description | `UpdateTeam` `PATCH /api/teams/{id}` |
| 停用 team | Organization admin 发起跨服务停用，保留历史 Team ID | `DeleteTeam` `DELETE /api/teams/{id}` |
| 列出成员 | 仅成员可读 | `ListMembers` `GET /api/teams/{id}/members` |
| 添加成员 | Organization admin 或 Team admin 添加已有企业成员，可指定 role | `AddMember` `POST /api/teams/{id}/members` |
| 调整成员 role | admin only；禁止把最后一名 admin 降级 | `UpdateMemberRole` `PATCH /api/teams/{id}/members/{userId}` |
| 移除成员 | admin 可移任何人；非 admin 仅可移自己；禁止移除最后一名 admin | `RemoveMember` `DELETE /api/teams/{id}/members/{userId}` |
| 列出邀请 | admin only；只返回未过期 | `ListInvites` `GET /api/teams/{id}/invites` |
| 签发邀请 | admin only；默认 7 天过期；返回明文 token（仅本次可见） | `CreateInvite` `POST /api/teams/{id}/invites` |
| 撤销邀请 | admin only；按 token 删 | `RevokeInvite` `DELETE /api/teams/{id}/invites/{token}` |
| 预览邀请 | 任意已登录 user 按 token 查 team 信息 + 自己是否已是成员 | `PreviewInvite` `GET /api/invites/{token}` |
| 接受邀请 | 同一 Organization 的成员按 token 入团；已是成员则幂等返回 | `AcceptInvite` `POST /api/invites/{token}/accept` |
| 内部：取成员关系 | 供 auth / 其他 domain 校验当前 user 对 team 的角色 | `getMembership(teamID, userID)` |
| 内部：取邀请 | 按 token 查邀请；供 preview / accept 共享 | `getInviteByToken(token)` |
| 查询部门功能 | 返回授权模式、显式配置和实时有效功能 | `GetEntitlements` `GET /api/teams/{id}/entitlements` |
| 设置部门功能 | 企业管理员原子设置 inherit/restricted 模式及 restricted 功能子集 | `SetEntitlements` `PUT /api/teams/{id}/entitlements` |

## 数据模型

### Tables — `teams`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | team id，应用层 `generateId()` 短随机串 |
| organization_id | text NOT NULL | 所属 Organization ID；创建后不可变 |
| name | text | team 名 |
| description | text | 描述，可空 |
| entitlement_mode | text NOT NULL | inherit/restricted；默认 inherit |
| status | text NOT NULL | active/decommissioning/decommissioned |
| created_by | text NOT NULL | 创建者 user id；关联 ``users.id``（创建者不级联删除；删 user 时由应用层处理（user 删除本身尚未建模）） |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### Tables — `team_entitlements`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| team_id | text NOT NULL | Team ID |
| feature_key | text NOT NULL | Organization 已获授权的功能标识 |
| granted_by | text NOT NULL | 执行授权的企业管理员 User ID |
| granted_at | timestamptz | 授权时间 |
| 复合主键 | — | `(team_id, feature_key)` |

### Tables — `team_members`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| team_id | text NOT NULL | team id；关联 ``teams.id` ON DELETE CASCADE`（team 删则成员清） |
| user_id | text NOT NULL | user id；关联 ``users.id` ON DELETE CASCADE`（user 删则成员清） |
| role | text NOT NULL | `admin` 或 `member` |
| joined_at | timestamptz | 入团时间 |
| 复合主键 | — | `(team_id, user_id)` |

### Tables — `team_invites`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| token | text PK | `tinv_<base64url>` 邀请 token；PK |
| team_id | text NOT NULL | 所属 team；关联 ``teams.id` ON DELETE CASCADE`（team 删则邀请清） |
| created_by | text NOT NULL | 签发人 user id；关联 ``users.id``（不级联删除（删 user 历史邀请留痕）） |
| expires_at | timestamptz | 过期时间，可空（空=不过期） |
| created_at | timestamptz | 签发时间 |

### UUID 生成规则
- `teams.id` = `generateId()` 短随机字符串（兼容存量），非自增 / 非 ulid；新的 API Token 管理 ID 使用 ULID。
- `team_invites.token` = `tinv_` 前缀 + 24 字节随机串 base64url 编码（约 32 字符，URL-safe），全局唯一。

## 能力详细说明

### CreateTeam
```
1. 入参：organization_id、name（必填，非空）、description（可选）
2. 校验 Organization active，当前 user 是该 Organization admin。
3. 事务：
   a. INSERT teams（id, organization_id, name, description, entitlement_mode='inherit', created_by）
   b. INSERT team_members（team_id=新 id, user_id=created_by, role='admin'）
4. 提交；返回 team + (my_role='admin', member_count=1)
```
> 创建者必须是已登录的 Organization admin；创建 Team 不复制 Organization Entitlement。

### GetTeam
```
1. team = getTeam(id)；不存在 → 404
2. 查询 Team Membership 与 Organization Membership。
3. Team 成员或 Organization admin 可读；其他调用者返回 404，避免枚举部门。
4. 返回 team 元数据 + my_role/organization_role + member_count。
```

### UpdateTeam / DeleteTeam
```
1. Update 允许 Organization admin 或 Team admin；Delete 仅允许 Organization admin，否则 403。
2. UpdateTeam：
   - 局部更新 name / description（任一字段缺省跳过）
   - 刷新 updated_at
   - 0 行 → 404
3. DeleteTeam：
   - 不直接物理删除；进入跨服务 Team Decommission Workflow
   - 先把 Team 标记为 decommissioning，阻止创建新 Workspace 和新邀请
   - 确认不存在非 deleted Workspace 后标记 decommissioned，并撤销未过期邀请
   - Team ID、Membership 历史和 Resource Grant 引用保留；decommissioned Team 不产生有效访问权限
```

### ListMembers / AddMember / UpdateMemberRole / RemoveMember
```
ListMembers:
  1. 校验调用者是 Team 成员或 Organization admin（否则 404）
  2. 返回全部成员（JOIN users 取 display_name，按 joined_at ASC）

AddMember:
  1. 校验当前 user 是 Organization admin 或 Team admin
  2. 校验目标 user_id 已是 Team 所属 Organization 的成员（否则拒绝）
  3. INSERT team_members ON CONFLICT (team_id,user_id) DO NOTHING（幂等）
  4. 失败（并发删 team 等） → 400

UpdateMemberRole:
  1. 校验当前 user 是 Organization admin 或 Team admin
  2. 若目标 role 非 'admin'：检查现有 admin 数量；若仅 1 名且是目标 → 409（避免末位 admin 被降级）
  3. UPDATE role；0 行 → 404

RemoveMember:
  1. 校验当前 user 是 Organization admin、Team admin 或 target.user_id == session.sub（self-remove）
  2. 取目标 membership；不存在 → 404
  3. 若目标 role == 'admin'：检查现有 admin 数量；若仅 1 名 → 409（"请改删 team"）
  4. DELETE team_members；0 行 → 404
```

### CreateInvite / ListInvites / RevokeInvite
```
CreateInvite:
  1. 校验当前 user role == 'admin'
  2. token = "tinv_" + base64url(randomBytes(24))
  3. INSERT team_invites（expires_at 默认 now+7d，可由 body.expires_in_days 覆盖）
  4. 返回 token 明文 + team_id + created_by + expires_at + created_at
     （token 明文仅本次响应可见，DB 不存）

ListInvites:
  1. 校验当前 user role == 'admin'
  2. SELECT WHERE team_id=? AND (expires_at IS NULL OR expires_at > now())
  3. JOIN users 取 created_by_name

RevokeInvite:
  1. 校验当前 user role == 'admin'
  2. DELETE WHERE token=?（不要求 team_id 匹配：token 唯一即可；防 race：token 校验由邀请路径统一负责）
```

### 内部 helper（不暴露 HTTP）
```
getMembership(teamID, userID) → TeamMembership?
  - 供 auth / 其他 domain 校验 user 在某 team 的 role
getTeam(teamID) → Team?
  - 供 service 内复查
listTeamsForUser(userID) → TeamSummary[]
  - 含 my_role + member_count
```

### 部门功能授权

1. `entitlement_mode=inherit` 时不复制企业授权，Team 的有效功能实时等于 Organization Entitlement。
2. `entitlement_mode=restricted` 时，有效功能为 `Organization Entitlement ∩ Team Entitlement`。
3. 只有 Organization admin 可以切换模式或修改 Team Entitlement；Team admin 不能扩大部门功能。
4. 企业撤销功能后，即使 `team_entitlements` 仍有旧记录，该功能也不再有效。
5. 查询接口返回 `entitlement_mode`、`configured_feature_keys` 和 `effective_feature_keys`；普通 Team 成员及 Organization admin 可读。
6. 设置接口由 Organization admin 调用。`restricted` 的显式集合必须是当前 Organization Entitlement 的子集；`inherit` 时清空显式集合，避免无效配置被误认为生效。
7. 设置模式与显式集合、递增 `authorization_version`、写入授权 Outbox 必须在同一数据库事务提交。

## 关联关系

### 依赖的 domain
- [auth](../0001-auth/auth.md) — 仅读 `users` 校验目标 user 存在（AddMember）；user id 全局来自 `users.id`，因此 team 复用 auth domain 的 user 概念，未单独建模 user 读取能力。
- [organization](../0009-organization/organization.md) — 提供企业归属、企业成员和 Organization Entitlement 上限。
- 本 domain 对 auth 不持会话——所有 HTTP handler 入口由 media_auth 的 session middleware 注入 `user.sub`（与其它 domain 同源）。

### 被依赖的 domain
- 当前 media_auth 内无下游 domain 依赖 team（team 是给其他微服务用的 ACL 基础数据）。
- 跨服务：media_materials / media_workflow 等会通过 `team_id` 做 Resource Grant 校验；此处仅暴露 stable 查询接口，**不做**资源可见性判定。

### 参与的 workflow
- [team_invite_accept](../../workflow/0004-team-invite-accept/team_invite_accept.md) — Preview / Accept 两条对外接口，内部完成 `getInviteByToken` + `addTeamMember` 幂等编排。
- [team_decommission](../../../workflow/0007-team-decommission/team_decommission.md) — 跨 Auth/Control 阻止新 Workspace，并在无活动 Workspace 后终止 Team。

### 关联术语
[Team](../../CONTEXT.md#team团队)、[Team Membership](../../CONTEXT.md#team-membership团队成员关系)、[Team Member Role](../../CONTEXT.md#team-member-role团队成员角色)、[Team Invite](../../CONTEXT.md#team-invite团队邀请)、[Last Admin Protection](../../CONTEXT.md#last-admin-protection末位管理员保护)、[Resource Grant](../../CONTEXT.md#resource-grant资源共享授权)
