# oauth domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/oauth-provider.ts` + `src/services/oauth-provider.ts` + `oauth_clients` / `oauth_authorization_codes` / `oauth_refresh_tokens` 表。
- `/api/oauth/token` 由 control_plane 的 form+JSON 双兼容，收窄为 IDL 强制 `application/x-www-form-urlencoded`（RFC 6749 §3.2）；JSON fallback 由 handler 手动解析、不在 IDL 表达。
- 本批次 IDL 不暴露 OAuth Client 注册/管理接口（client 由脚本/控制面维护，沿用 control_plane 现状）。
- 三 grant（authorization_code + PKCE S256 / refresh_token 轮转 / RFC 8693 token-exchange）语义不变；定时清理沿用。
