---
name: team
description: 团队与团队成员管理、邀请链接签发与撤销；为其他服务提供 team 维度 ACL 基础数据
dependencies:
    - auth
related_workflow:
    - team_invite_accept
---

## 一句话职责
team domain 维护 team 的创建/查询/更新/删除、user↔team 的成员关系（含角色）、以及 admin 签发/撤销邀请链接；为 [Resource Grant](../../CONTEXT.md#resource-grant资源共享授权) 之类的资源级 ACL 提供 `team_id` 维度。

## 边界

### 做什么
- team 自身 CRUD：创建（创建者自动成为首任 admin）、查询、更新（name/description）、删除（级联清掉成员与邀请）。
- 成员管理：列出 team 成员、加入（admin 主动 add 或邀请接受）、降级/升级 role、移除（含 self-remove）。
- 邀请链接：admin 签发带过期时间的 `tinv_` 随机 token；admin 撤销；列出未过期邀请。
- 内部只读查询：按 user 列出可见 team、按 token 查邀请（供预览/接受）；供 [auth](../0001-auth/auth.md) 等 domain 用 `getMembership(teamID, userID)` 做权限判定。

### 不做什么
- 不做资源级 ACL 业务（`visibility='team'`、Resource Grant 校验）——team 仅暴露 team_id 与成员关系，下游服务（media_materials / media_workflow）按需取用。
- 不签发 OAuth Client / 不接管 Session——分别归 [oauth](../0005-oauth/oauth.md) / [auth](../0001-auth/auth.md)。
- 不管理 service token、identity 绑定、企微登录通道——分别归 [service_token](../0004-service_token/service_token.md) / [identity](../0002-identity/identity.md) / [wecom](../0003-wecom/wecom.md)。
- 不实现跨 workspace 的 teamwork 协作（多 workspace + AFS 共享）——那是 control-plane 现存 `teamwork` 域，与本 team domain 不同概念；本服务不迁 teamwork。
- 不做 billing、不做租户隔离——team 只是 user 之上、组织 user 的命名分组。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建 team | 建团，创建者自动加入为唯一 admin；事务内插 team + team_member | `CreateTeam` `POST /api/teams` |
| 列出我所在的 team | 按当前 user 查所有 membership，含 my_role / member_count | `ListMyTeams` `GET /api/teams` |
| 查看 team 详情 | 仅成员可读；返回 team 元数据 + 当前 user 的 my_role + member_count | `GetTeam` `GET /api/teams/{id}` |
| 修改 team | admin only；name / description 局部更新 | `UpdateTeam` `PATCH /api/teams/{id}` |
| 删除 team | admin only；级联清 team_members + team_invites | `DeleteTeam` `DELETE /api/teams/{id}` |
| 列出成员 | 仅成员可读 | `ListMembers` `GET /api/teams/{id}/members` |
| 添加成员 | admin only；可指定 role（默认 `member`）；目标 user 必须存在 | `AddMember` `POST /api/teams/{id}/members` |
| 调整成员 role | admin only；禁止把最后一名 admin 降级 | `UpdateMemberRole` `PATCH /api/teams/{id}/members/{userId}` |
| 移除成员 | admin 可移任何人；非 admin 仅可移自己；禁止移除最后一名 admin | `RemoveMember` `DELETE /api/teams/{id}/members/{userId}` |
| 列出邀请 | admin only；只返回未过期 | `ListInvites` `GET /api/teams/{id}/invites` |
| 签发邀请 | admin only；默认 7 天过期；返回明文 token（仅本次可见） | `CreateInvite` `POST /api/teams/{id}/invites` |
| 撤销邀请 | admin only；按 token 删 | `RevokeInvite` `DELETE /api/teams/{id}/invites/{token}` |
| 预览邀请 | 任意已登录 user 按 token 查 team 信息 + 自己是否已是成员 | `PreviewInvite` `GET /api/invites/{token}` |
| 接受邀请 | 任意已登录 user 按 token 入团；已是成员则幂等返回 | `AcceptInvite` `POST /api/invites/{token}/accept` |
| 内部：取成员关系 | 供 auth / 其他 domain 校验当前 user 对 team 的角色 | `getMembership(teamID, userID)` |
| 内部：取邀请 | 按 token 查邀请；供 preview / accept 共享 | `getInviteByToken(token)` |

## 数据模型

### Tables — `teams`
| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| id | team id，应用层 `generateId()` 短随机串 | text PK | — | 全局唯一 |
| name | team 名 | text | — | 必填 |
| description | 描述，可空 | text | — | — |
| created_by | 创建者 user id | text NOT NULL | `users.id` | 创建者不级联删除；删 user 时由应用层处理（user 删除本身尚未建模） |
| created_at | 创建时间 | timestamptz | — | — |
| updated_at | 更新时间 | timestamptz | — | UpdateTeam 时刷新 |

### Tables — `team_members`
| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| team_id | team id | text NOT NULL | `teams.id` ON DELETE CASCADE | team 删则成员清 |
| user_id | user id | text NOT NULL | `users.id` ON DELETE CASCADE | user 删则成员清 |
| role | `admin` 或 `member` | text NOT NULL | — | DB CHECK 约束 |
| joined_at | 入团时间 | timestamptz | — | — |
| 复合主键 | `(team_id, user_id)` | — | — | — |

### Tables — `team_invites`
| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| token | `tinv_<base64url>` 邀请 token；PK | text PK | — | 一次性（接受成功后保留供溯源；不删除；如要"一次性"语义须由额外字段实现——当前 control-plane 仅按过期/撤销判失效） |
| team_id | 所属 team | text NOT NULL | `teams.id` ON DELETE CASCADE | team 删则邀请清 |
| created_by | 签发人 user id | text NOT NULL | `users.id` | 不级联删除（删 user 历史邀请留痕） |
| expires_at | 过期时间，可空（空=不过期） | timestamptz | — | 应用层判 expired |
| created_at | 签发时间 | timestamptz | — | — |

### UUID 生成规则
- `teams.id` = `generateId()` 短随机字符串（与 `users.id` / `service_tokens.id` 同机制），非自增 / 非 ulid / 兼容存量。
- `team_invites.token` = `tinv_` 前缀 + 24 字节随机串 base64url 编码（约 32 字符，URL-safe），全局唯一。

## 能力详细说明

### CreateTeam
```
1. 入参：name（必填，非空）, description（可选）
2. 事务：
   a. INSERT teams（id, name, description, created_by）
   b. INSERT team_members（team_id=新 id, user_id=created_by, role='admin'）
3. 提交；返回 team + (my_role='admin', member_count=1)
```
> 创建者必须是已登录 user（sub 来自 session）；不开放匿名建团。

### GetTeam
```
1. team = getTeam(id)；不存在 → 404
2. membership = getMembership(id, session.sub)；不存在 → 404（不暴露 team 是否存在给非成员）
3. 返回 team 元数据 + membership.role + listMembers(id).length
```

### UpdateTeam / DeleteTeam
```
1. 校验 membership.role == 'admin'，否则 403
2. UpdateTeam：
   - 局部更新 name / description（任一字段缺省跳过）
   - 刷新 updated_at
   - 0 行 → 404
3. DeleteTeam：
   - DELETE teams WHERE id=?；DB ON DELETE CASCADE 清 team_members + team_invites
   - 0 行 → 404
```

### ListMembers / AddMember / UpdateMemberRole / RemoveMember
```
ListMembers:
  1. 校验 membership 存在（非成员 → 404）
  2. 返回全部成员（JOIN users 取 display_name，按 joined_at ASC）

AddMember:
  1. 校验当前 user role == 'admin'
  2. 校验目标 user_id 存在（不存在 → 400）
  3. INSERT team_members ON CONFLICT (team_id,user_id) DO NOTHING（幂等）
  4. 失败（并发删 team 等） → 400

UpdateMemberRole:
  1. 校验当前 user role == 'admin'
  2. 若目标 role 非 'admin'：检查现有 admin 数量；若仅 1 名且是目标 → 409（避免末位 admin 被降级）
  3. UPDATE role；0 行 → 404

RemoveMember:
  1. 校验当前 user role == 'admin' 或 target.user_id == session.sub（self-remove）
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

## 关联关系

### 依赖的 domain
- [auth](../0001-auth/auth.md) — 仅读 `users` 校验目标 user 存在（AddMember）；user id 全局来自 `users.id`，因此 team 复用 auth domain 的 user 概念，未单独建模 user 读取能力。
- 本 domain 对 auth 不持会话——所有 HTTP handler 入口由 media_auth 的 session middleware 注入 `user.sub`（与其它 domain 同源）。

### 被依赖的 domain
- 当前 media_auth 内无下游 domain 依赖 team（team 是给其他微服务用的 ACL 基础数据）。
- 跨服务：media_materials / media_workflow 等会通过 `team_id` 做 Resource Grant 校验；此处仅暴露 stable 查询接口，**不做**资源可见性判定。

### 参与的 workflow
- [team_invite_accept](../../workflow/0004-team-invite-accept/team_invite_accept.md) — Preview / Accept 两条对外接口，内部完成 `getInviteByToken` + `addTeamMember` 幂等编排。

### 关联术语
[Team](../../CONTEXT.md#team团队)、[Team Membership](../../CONTEXT.md#team-membership团队成员关系)、[Team Member Role](../../CONTEXT.md#team-member-role团队成员角色)、[Team Invite](../../CONTEXT.md#team-invite团队邀请)、[Last Admin Protection](../../CONTEXT.md#last-admin-protection末位管理员保护)、[Resource Grant](../../CONTEXT.md#resource-grant资源共享授权)