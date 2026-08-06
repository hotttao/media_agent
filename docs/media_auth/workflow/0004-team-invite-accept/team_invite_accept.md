---
name: team_invite_accept
desc: 团队邀请的预览与接受——任意已登录 user 按 token 完成入团；接受操作对已入团者幂等
---

## 一句话说明
非 team 成员的已登录 user 持 `tinv_` token 调 `/api/invites/{token}` 预览、按 `/api/invites/{token}/accept` 入团；流程完全发生在 team domain 内部（不跨其它 domain），但因同时承担"邀请持有者"与"team 内 admin"两种身份且对外为两条独立 HTTP 路径，符合 workflow 跨域或跨身份编排的判定（即便此处未跨域），按 workflow 文档化便于 P2 反代对齐。

## 执行时间线（每个接口内嵌内部处理）

```text
invitee               media_auth (team domain)                       users (auth domain)
   |                          |                                          |
   |-- GET /api/invites/{token} ----->|                                 |
   |   PreviewInvite                  |                                 |
   |   ├─ invite = getInviteByToken(token)                              |
   |   │    无 → 404                                                    |
   |   │    过期 → 410                                                  |
   |   ├─ team = getTeam(invite.team_id)                                 |
   |   │    无 → 404（team 已删，邀请残魂）                              |
   |   ├─ inviter = getUser(invite.created_by)  ← 只读 auth              |
   |   ├─ membership = getMembership(team.id, session.sub)               |
   |   └─ 返回 {team_id, team_name, inviter_name,                       |
   |            expires_at, already_member: !!membership}                |
   |<-- 200 ----|                                                        |
   |                          |                                          |
   |-- POST /api/invites/{token}/accept ->|                              |
   |   AcceptInvite                   |                                  |
   |   ├─ invite = getInviteByToken(token)                              |
   |   │    无 → 404                                                    |
   |   │    过期 → 410                                                  |
   |   ├─ team = getTeam(invite.team_id)                                 |
   |   │    无 → 404                                                    |
   |   ├─ existing = getMembership(team.id, session.sub)                 |
   |   │    存在 → 返回 {team_id, already_member: true}  ← 幂等         |
   |   ├─ addTeamMember(team.id, session.sub, role='member')            |
   |   │    INSERT ... ON CONFLICT (team_id,user_id) DO NOTHING         |
   |   │    ← 无 INSERT 抛错；冲突（并发另一端也加）静默                  |
   |   └─ 返回 {team_id, already_member: false}                          |
   |<-- 200 ----|                                                        |
```

## 关键流程参数传递
- `token`：路径参数。`tinv_<base64url>` 格式；唯一识别一次邀请。**token 不带 user 绑定**——任何已登录 user 持 token 都能预览/接受；这是 control-plane 现行语义（链接即授权），media_auth 沿用。如需"只能特定 user 接受"，须在 invite 上加 `created_for` 字段，留待后续按需建模。
- `session.sub`：所有 handler 入口由 media_auth session middleware 注入；invitee 必须已登录（cookie/bearer 任一），否则 middleware 直接 401。
- 入团 `role`：固定 `'member'`；不允许通过 invite 直升 admin。admin 任命由 `PATCH /api/teams/{id}/members/{userId}` 单独走。
- 不下发新 cookie：用户已是 platform auth 用户，session 不变；入团后通过 `ListMyTeams` 自然可见该 team。

## 状态流转
invite 的状态由"是否存在 + 是否过期"决定，无显式 status 字段：
```mermaid
stateDiagram-v2
    [*] --> Active: CreateInvite (expires_at = now+7d)
    Active --> Active: 未到 expires_at
    Active --> Expired: now() > expires_at
    Active --> Revoked: admin DELETE token
    Expired --> [*]
    Revoked --> [*]
    note right of Active
      已过期/撤销后，
      Preview/Accept 一律 404 / 410；
      不删行，留作审计。
    end note
```

## 幂等设计
- **AcceptInvite 幂等**：已入团者再点接受 → 直接 `already_member=true` 返回，不写库、不报错。
- **并发接受**：两个 tab 同时 Accept → 两边都过 `existing` 检查后都走 `addTeamMember`，ON CONFLICT 保证一行；两边都返回 `already_member=false` 但 DB 只多一行（无副作用）。
- **过期竞态**：`Preview` 通过后、`Accept` 触发前 token 刚过期 → Accept 内部重检 `expires_at` → 返回 410；不会写入半状态。
- **team 已删**：invite 因 `ON DELETE CASCADE` 已被清，`getInviteByToken` 返回 null → 404；不会指向幽灵 team。

## 一致性边界

Accept 在一个本地事务中重新读取并锁定 Invite、检查有效期与 Team 状态、插入 Team Membership。Preview 是非授权只读提示，不能替代 Accept 内的检查。通知或审计发布通过 Outbox，失败不回滚已建立的成员关系。

## 权限与失败

- 当前模型是 capability link：持有 Invite Token 的任意已登录用户可以接受，Token 不代表创建者身份。
- Cookie JWT 或用户 Service Token 决定接受者，body/path 不允许覆盖 user_id；角色固定为 member。
- Invite 不存在、撤销、过期或 Team 已删除不可重试；并发接受由唯一约束收敛为同一 Membership。
- Token 不得进入访问日志和 Trace；目标实现只保存高强度随机 Token 的 Hash，预览/接受时 Hash 查询。
- **invite 无一次性消费**：control-plane 现状是接受后 token 仍留痕（审计 / 防止合法邀请被反复 retry）。如业务要求"一次接受即失效"，需在 invite 上加 `consumed_at` 字段 + 接受时 UPDATE；当前不引入，留待按需建模。
