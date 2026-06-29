# auth domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/auth.ts` + `src/services/auth.ts` + `users` 表。
- session（JWT cookie 签发/校验/滑动续期）归入本 domain，不再散落在 control_plane 顶层中间件。
- 行为修正：`auth_source` 对企微派生账号由 control_plane 的误报 `ldap` 改为 `wecom`（需只读 identity 推导，见 auth.md）。
- 行为变更：`ApiUser` 移除 `default_prompt_name`；`default_prompt_id` 存为不校验 FK 的 opaque text，name 由网关补（prompts 表留在 control_plane，见 ADR-0001）。
- 登录后 `ensurePlatformToken` 委托 service_token domain 内部能力，语义不变。
