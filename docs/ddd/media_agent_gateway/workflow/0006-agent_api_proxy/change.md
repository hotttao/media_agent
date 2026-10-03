# Agent API Proxy 变更记录

## 2026-10-03 · 独立委托签名信任链

- 明确 Gateway 的独立签名 issuer、单服务 audience 和60秒请求摘要绑定，业务服务保留 Agent Actor而不降级为User。
- 原始 Token只用于Auth验证；代理固定服务/路由，写请求不自动重试，不复用Oathkeeper私钥或Worker认证分支。
