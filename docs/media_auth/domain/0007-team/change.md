# team domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`agent-platform/control-plane/src/routes/teams.ts` + `src/routes/invites.ts` + `src/services/db/teams.ts` + `src/services/db/team-invites.ts` + `migrations/001_init.sql` 中 `teams` / `team_members` / `team_invites` 三表。
- 业务语义与 control-plane 完全对齐：team 是 user 之上、组织 user 的命名分组，仅作 Resource Grant ACL 基础数据，不承担多租户隔离、不承担 billing。teamwork（多 workspace 协作 + AFS 共享）属 control-plane 单独的域，本服务不迁。
- 行为保留：末位 admin 保护（UpdateMemberRole / RemoveMember 拒绝把唯一 admin 降级或移除）；AddMember / AcceptInvite 幂等；invite token 仅创建时返回明文，DB 不存。
- 接口拆分：原 control-plane `/api/invites/*` 的 Preview / Accept 两条路径在 media_auth 中保留（不合并进 `/api/teams/{id}/invites/*`）—— 前者是"任意已登录 user 按 token 操作"，后者是"team 内 admin 管理邀请"。两条路径虽都涉及 invite 实体，但操作者与权限模型不同，合并反而把 admin 鉴权与 invite-持有者鉴权混在一起。
- 协议路径与字段命名沿用 control-plane 现状（`/api/teams`、`/api/invites`、snake_case `my_role` / `member_count` 等），便于 P2 control_plane 网关反代时前端零改动。
- 暂不引入新能力：team 合并 / 拆分 / 转让 ownership / 邀请一次性消费（接受后立即失效 token）—— 留待后续按需建模。