# 2026-08-06

将 Platform Token 从 `service_token` 聚合拆出；本 domain 只保留用户创建的 CLI/API Service Token。内部验证术语统一为 `IntrospectServiceToken`，返回 Principal。

## 2026-09-27 · Media Worker 凭证边界

- “API Token”统一为 Service Token 的客户端用途，不新建凭证聚合。
- Worker 用 Token 必须绑定唯一 Organization，并通过 scopes 限制 poll、heartbeat、complete、fail 与 upload。
- Introspection 返回业务 User ID、Organization、credential_id 与 scopes，Edge 再签发短时 Internal Identity JWT；Worker 路由不依赖 Browser Cookie。
- Worker Instance 只用于租约和审计，不作为新的授权主体。
