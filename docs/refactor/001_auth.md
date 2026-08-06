# 001 认证与鉴权规范

## 1. 决策

当前平台内服务均由平台控制部署，采用同一信任域内的 Trusted Subsystem 模型：

- 公网凭证在 Edge 终止。
- 内部每一跳使用 mTLS 认证直接调用服务。
- 需要代表在线用户时传递受控 `Principal` 上下文。
- 不需要用户语义时只使用 mTLS。
- `platform_token` 仅用于离线长期委托和少数旧式服务。
- 认证方式由目标接口规定，调用方不能自行降级选择。

当前不引入每跳 OAuth Token Exchange。未来允许不受信任的第三方后端或跨信任域部署时再评估短时、audience 受限 Token。

## 2. 两类身份

```text
CallerService：直接调用方，由 mTLS 证书 SAN 得到。
Principal：本次操作代表的用户、Workspace 或 Agent Session。
```

统一应用上下文：

```go
type RequestContext struct {
    CallerService string
    Principal     Principal
    RequestID     string
}

type Principal struct {
    Kind        string
    AuthMethod  string
    UserID      string
    WorkspaceID string
    SessionID   string
    AgentID     string
    TokenID     string
    Scopes      []string
}
```

`CallerService` 禁止从 Header、Body 或 Query 获取。

## 3. 凭证与入口

| 凭证 | 使用入口 | 验证者 | 结果 |
|---|---|---|---|
| Cookie JWT | Web -> Edge | Edge 使用 Auth JWKS | User Principal |
| 用户 Service Token | CLI -> Edge | Edge 调 Auth introspection | User Principal |
| Session Token | Agent -> MCP | Gateway `ResolveSessionContext` | Agent Session Principal |
| Platform Token | 可信内部调用 | mTLS allowlist + Auth introspection | User Principal |
| 资源分享 Token | 资源专用公开路由 | 资源所有者服务 | Resource Grant |
| mTLS Certificate | 内部每一跳 | 目标服务 | CallerService |

公共 Edge 拒绝 Session Token 和 Platform Token。业务服务不接收原始 Cookie 或用户 Service Token。

## 4. 接口认证模式

| 模式 | 认证要求 | 示例 |
|---|---|---|
| `PUBLIC_EDGE_USER` | Edge mTLS + Edge User Header | Web/CLI API |
| `INTERNAL_SERVICE` | 调用服务 mTLS | Gen -> Storage |
| `INTERNAL_DELEGATED_USER` | mTLS + caller allowlist + Principal | Workflow -> Gen |
| `INTERNAL_PLATFORM_TOKEN` | mTLS + caller allowlist + Platform Token | Gateway -> Browser |
| `AGENT_SESSION` | Agent mTLS + Session Token | Agent -> 业务 MCP |
| `RESOURCE_GRANT` | 资源本地 Token | Browser 分享链接 |

Public、Internal 和 MCP Listener 分离。每个 Internal RPC 必须声明允许的调用服务。

## 5. 在线长链路

```text
UI --Cookie--> Edge
Edge --mTLS + authenticated user headers--> Workflow
Workflow --mTLS + DelegatedPrincipal--> Media Gen
Media Gen --mTLS--> Storage
```

Edge 删除客户端传入的所有内部身份 Header，验证 Cookie/Service Token 后写入标准 User Header。Workflow 只在 mTLS caller 为 Edge 时信任这些 Header。

内部 RPC 使用 IDL 明确定义委托上下文：

```proto
message DelegatedPrincipal {
  string user_id = 1;
  string workspace_id = 2;
  string session_id = 3;
  string agent_id = 4;
  string source = 5;
  string source_resource_id = 6;
}
```

目标服务从 mTLS 取得 caller，校验接口 allowlist，再使用 Principal 执行 Workspace 和本地资源鉴权。`DelegatedPrincipal` 是受信服务声明，不是授权结果。

## 6. Agent 与离线任务

平台内业务 MCP：

```text
Agent -> session_token -> 业务 MCP
业务 MCP -> ResolveSessionContext -> user/workspace/session/agent
业务 MCP -> 同进程 Application Service
```

Platform MCP：

```text
Agent -> session_token -> media_agent_gateway
Gateway -> 必要时取得 platform_token -> Browser/Sandbox
```

异步 Workflow Job 只保存业务 ID，Worker 从业务记录恢复 `requested_by` 和 Workspace，再使用 mTLS + `DelegatedPrincipal` 调下游。

Agent Schedule 属于长期离线委托：Schedule Worker 执行时按 purpose 从 Auth 获取 Platform Token，不把 Token 写入数据库任务、River、Outbox、日志或 Trace。

## 7. 授权责任

```text
Edge：凭证有效、路由可进入。
media_control：Workspace 状态、owner 和 Workspace action。
media_auth：User/Team/TeamMembership。
资源服务：资源归属、状态和具体 action。
```

服务必须从数据库读取资源真实 `workspace_id`，不能信任客户端或上游声明的资源归属。列表使用 BatchCheck 或允许集合过滤，禁止逐行授权 RPC。

## 8. Token 和故障约束

- Session Token 与旧 `agent-platform` 行为一致：一个 Session 一个稳定 opaque Token，后续 Turn 复用。
- Platform Token 由 `media_auth` 加密保存；只有 Gateway 和 Schedule Worker 的明确身份可按 purpose 获取。
- Session Resolve 正缓存最多 60 秒、负缓存最多 5 秒；cache miss 且 Gateway 不可用时拒绝。
- 用户 Service Token introspection 使用相同缓存上限；cache miss 且 Auth 不可用时拒绝。
- mTLS 在 local/test/dev 可关闭，staging/prod 关闭时拒绝启动。
- Token、Cookie 和分享 URL 必须在日志、Trace 和错误报告中脱敏。
- 权限撤销后禁止新调用；已经开始的调用允许自然结束。
