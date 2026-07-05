# team_invite_accept workflow 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`agent-platform/control-plane/src/routes/invites.ts`（`PreviewInvite` / `AcceptInvite`）。
- 编排范围判断：流程仅读 `team_invites` / `teams` / `users` / `team_members`，完全落在 team domain + auth domain 的只读查询范围内，未跨域编排其它业务对象。之所以仍按 workflow 记录，是因为：① 它对外是两条独立 HTTP 路径（`/api/invites/{token}` 与 `/api/invites/{token}/accept`），具备端到端生命周期；② token 与 session 的关联判定是典型的"跨身份"语义（admin 签发 ↔ invitee 接受），涉及鉴权主体两次切换，把规则沉淀在 domain 文档会掩盖鉴权差异。
- 鉴权差异：admin 创建/撤销走 `team.GetMembership(...).role == 'admin'`（路径 `/api/teams/{id}/invites`），invitee 接受走 session 任意已登录 user（路径 `/api/invites/{token}`），不要求是 team 成员——两条路径权限模型不同，文档分开记录避免下游误以为 invite 走 admin 鉴权。
- 行为保留：`isExpired` 判定、`already_member` 幂等返回、ON CONFLICT DO NOTHING 的并发安全语义与 control-plane 完全一致。
- 协议沿用 control-plane 现状：`/api/invites/{token}` (GET) + `/api/invites/{token}/accept` (POST)；P2 control_plane 网关反代时前端无需改动。