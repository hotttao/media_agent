---
name: agent_api_token_provisioning
description: 创建 Agent 时为其申请绑定 owner User 的一次性可见 API Token
participants: [browser, media_agent_gateway, media_control, media_auth, talos]
domains: [agent_configuration, api_token]
---

## 一句话说明

用户创建 Agent 后，media_agent_gateway 以受信服务身份请求 media_auth 为 `Agent:<agent_id>` 创建 API Token；media_auth 验证 Agent、owner 与 Organization 后调用 Talos，完整 Secret 只沿同步响应返回一次。

## 执行时间线

```mermaid
sequenceDiagram
    participant U as User Browser
    participant G as media_agent_gateway
    participant C as media_control
    participant A as media_auth
    participant T as Talos
    participant DB as Auth DB
    U->>G: Create Agent + request API Token (Web Session)
    G->>C: Create/Get Agent Configuration
    C-->>G: agent_configuration_id + owner_user_id + organization_id
    G->>A: CreateActorAPIToken(configuration ID, scopes, idempotency key; trusted owner header)
    A->>C: ResolveAgentIdentity(agent_configuration_id), verify owner/org
    C-->>A: authoritative relationship
    A->>DB: TX: issuing metadata + stable issuance intent
    A->>T: Create API Key(actor_id=Agent:configuration_id, request_id=token_id)
    T-->>A: talos_key_id + one-time Secret
    A->>DB: TX: CAS issuing to active + key metadata + audit
    alt Auth TX committed
        A-->>G: token metadata + one-time Secret
        G-->>U: Agent + one-time Secret
    else Auth TX failed
        A->>DB: TX: issuance_cleanup intent
        A->>T: revoke key; persistent issuance recovery if uncertain
        A-->>G: creation failed; Secret discarded
        G-->>U: Agent created, Token provisioning requires retry
    end
```

Agent Configuration 由 media_control 持有，API Token Metadata 由 media_auth 持有，Talos 持有 Secret。Gateway 只协调，不保存 Secret。Agent 创建成功但 Token 创建失败时不回滚 Agent；返回明确的 `token_provisioning_failed`，用户可以对同一 Agent 使用稳定幂等键重试。

稳定 Actor 是 `Agent:<AgentConfiguration.id>`，现有 `agent_id` 是 Runtime 类型标签。Auth 通过 Control 内部身份只读投影取得 Workspace/owner/企业/部门及当前授权，不能通过 Runtime Context 获取 Provider Secret。停止运行不等于身份删除；独立停用/解绑及所有 Token 的撤销必须由生命周期 Workflow 保证。签发直接复用已有 API Token Issuance 的持久意图、CAS 与未知结果恢复，禁止另建“先签发、再写库”的易丢失补偿路径。

## 幂等、恢复与生命周期

- 创建者范围内的幂等键为 `agent:` 加配置 ID、固定 purpose `agent-gateway`、客户端请求键经 NUL 分隔后的 SHA-256 十六进制摘要；客户端请求键必须非空且不超过 200 字符。重复已有有效签发意图返回 409，不重放 Secret，也不隐式轮换。
- 同步响应丢失时，通过 Token 管理查询确认元数据；Secret 无法恢复。必须显式轮换或重新申请，不因断线自动生成第二把 Key。签发状态、事务和未知结果恢复遵循 [API Token Issuance](../../../media_auth/workflow/0010-api-token-issuance/api_token_issuance.md)。
- Talos 创建成功而 Auth 事务失败时立即撤销 Key；撤销结果不明确写补偿任务，禁止把 Secret 交给调用方。
- Agent disabled/deleted 或 owner 关系解除后，Gateway/Control 发布事实，media_auth 幂等撤销该 Agent 的全部 active API Token。
- 轮换生成新 Secret并撤销旧 Key；旧 Secret 不可恢复。撤销失败保留 `revoking` 状态并重试，运行时验证 fail closed。

## 权限与信任边界

Web Session 决定创建者 User；请求体中的 owner、Organization 和 Agent ID 只能作为目标引用，必须回查 media_control。Gateway 到 Auth、Auth 到 Control/Talos 使用受信服务通道。Agent Token scope 不得超过 User 在 Organization 内可委托权限；第一阶段 Agent 可继承 User 权限上限，但资源服务仍按当前 User、Actor、Organization 和资源事实决定最终授权。

Auth 的签发入口为私有 `POST /internal/v1/agent-api-tokens`。Gateway 使用专用 `X-Agent-Gateway-Internal-Token` 与可信 `X-Media-Auth-Acting-User-ID`；专用凭据未配置即拒绝，普通内部查询凭据不能授权签发。请求体只提供配置引用和 Token 参数，不能覆盖 owner、Actor 或 Organization。Auth 回查 Control 的当前归属及授权，派生所有身份字段；第一阶段只允许 `agent.gateway` scope，空数组默认该 scope，拒绝 `api.access` 和 Worker scope。停止 Runtime 不使配置身份失效，删除或失去 owner 授权则拒绝。

非法参数返回 400、身份凭据缺失/错误返回 401、归属或授权不符返回 403、未知配置返回 404、重复签发返回 409；权威服务不可用或持久签发失败返回 503。失败响应不含 Secret 或上游敏感错误正文，成功响应禁止缓存。此入口不加入公开网关路由。

Agent 运行时只调用 media_agent_gateway。Gateway 按全局 API Token Verification 流程形成 `sub=User:<owner>`、`act.sub=Agent:<id>` 的 Internal Identity JWT 后代理内部服务；API Token 不直接传播给内部服务。

## 结果

成功产生一个 active API Token Metadata 和 Talos Key，Agent/User 仅本次获得 Secret；失败不会留下可用但无管理记录的 Key。该流程不创建 Agent Session，Session 生命周期由 Agent Session Startup 单独管理。
