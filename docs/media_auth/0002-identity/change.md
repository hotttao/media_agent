# identity domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/wecom-auth.ts` 中的 identities list/delete 段 + `user_identities` 表。
- 把 list/delete 从写死 `/api/auth/wecom/*` 泛化为按 provider 路由：`GET /api/auth/identities`、`DELETE /api/auth/identities/:provider`。
- 破坏性变更：原 `DELETE /api/auth/wecom/identity`（无 provider 参数）改为带 provider path 参数；P2 前端需适配。
- 新增内部能力 `findUserIdentity` / `bindIdentity`，供 wecom 登录通道复用，避免 wecom 直读 identity 表。
