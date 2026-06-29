# service_token domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/service-tokens.ts` + `src/lib/service-token.ts` + `service_tokens` 表。
- Platform Token 明文 `token` 列保留，见 ADR-0002。
- `ensurePlatformToken` / `getPlatformToken` / `getServiceTokenByHash` 收口为本 domain 内部能力；control_plane 不再直读 `service_tokens` 表，改经 media_auth internal `VerifyToken` RPC（见 service_token_verification workflow）。
- 撤销接口 `WHERE is_platform = false` 不变，Platform Token 仍不可经公开 API 撤销。
