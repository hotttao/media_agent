---
name: agent_api_proxy
description: 以可核验的 Agent 委托身份代理固定内部业务 API
participants: [agent_client, media_agent_gateway, media_auth, media_control, business_service]
domains: [api_token, agent_configuration]
---

## 一句话说明

Agent 携带自己的 API Token 请求 Gateway；Gateway 每次重新验证凭据及委托关系，以独立签名、绑定目标请求的短时 Internal Identity JWT 调用业务服务，后者保留 Actor 并执行当前资源授权。

## 执行时间线

```mermaid
sequenceDiagram
    participant C as Agent Client
    participant G as Agent Gateway
    participant A as media_auth / Talos
    participant R as Control Agent Identity
    participant B as Business Service
    C->>G: API Token + declared target operation
    G->>G: Validate fixed service/route/method allowlist and request limits
    G->>A: VerifyAPIToken(opaque Secret; trusted query credential)
    A-->>G: Current User/Agent/credential/org/scope or inactive
    G->>R: Resolve stable configuration identity
    R-->>G: Current owner/org/Workspace/Team/authorization
    G->>G: Match Agent identity; sign 60s JWT for one target request
    G->>B: Trusted JWT + trace context; no external identity headers/Cookie
    B->>B: Verify dedicated issuer/audience/expiry/Actor/request digest
    B->>B: Authorize current User/Organization/Workspace/resource
    B-->>G: Business status and result
    G-->>C: Result; no JWT/Secret echoed
```

Gateway 不创建 Token、Job 或资源事实；目标写事务完全由资源所属服务控制。JWT 只是经核验的一次委托断言，不是资源权限快照。停止 Runtime 不使身份自动失效；已删除或失去 owner 授权则拒绝。Agent 身份与 Session Registration/Worker 身份独立。

## 身份与信任边界

- Agent Gateway 使用自己的 RSA 签名密钥和固定 issuer，不读取或复用 Oathkeeper 私钥。JWKS 只发布公钥；签名密钥仅由 Gateway 持有，未配置时代理关闭。
- Subject 为 `User:<owner>`，Actor 为 `Agent:<configuration_id>`；保留 credential_id、organization_id 和 `agent.gateway` scope，auth_source 为 `agent_api_token`，不继承客户端 Role 或 acting-user Header。
- audience 只包含本次目标服务；有效期60秒，iat/nbf/exp/jti均必填。请求摘要覆盖 HTTP method、原始 Request URI（含 query）和原始 body；不能转用于其他路径、参数或载荷。它不代替业务幂等键，不能声称防止同请求重放。
- 目标服务仅在单独配置的 Agent verifier 信任链上接收此身份，普通 Oathkeeper User Token 和 Personal Worker 分支保持独立。不因新增委托接入放行普通 JWT 的 Agent Actor。
- 内部请求只构造签名 Authorization、Content-Type 和 Trace Context，不传原始 API Token、Cookie、Host、转发或内部服务凭据。目标固定于服务端配置及路由 allowlist；禁止任意 URL、内部/认证/Worker/运维路由及重定向跳转。
- 目标服务将 User 与 Agent Principal 同时保留在请求上下文和审计维度。资源授权依旧回查当前事实，素材企业列表限于 JWT 企业；Workflow Workspace 必须属于被委托企业。

## 一致性、重试与失败

无 Gateway 持久化状态流转或分布式事务。读取与写命令都只转发一次，不自动重试写请求。目标超时或客户端断线不回滚已提交的资源事实；调用者使用资源服务的幂等键或查询恢复，不能因请求失败再次创建 Job。

| 情况 | 可观察结果 | 恢复 |
|:---|:---|:---|
| 无效/过期/撤销 Token | 401 | 由 User 管理凭据 |
| 委托关系/企业/scope不符 | 403 | 修复当前授权，不签 JWT |
| 权威不可用 | 503 | 无业务调用，不用缓存身份降级 |
| 非允许目标或过大请求 | 400/413 | 修正请求，不接触业务服务 |
| JWT验证/请求摘要不符 | 401 | 拒绝，不退回 Cookie或本地Hash |
| 目标拒绝资源授权 | 保留业务拒绝 | 不提升为owner/admin |
| 目标结果不明 | 安全的网关错误 | 查业务结果；不重发、补偿或自动取消 |

## 结果事件

无。业务写入产生的事件归目标 Domain；Gateway 只记录无 Secret/JWT/body 的调用审计和 Trace Context。
