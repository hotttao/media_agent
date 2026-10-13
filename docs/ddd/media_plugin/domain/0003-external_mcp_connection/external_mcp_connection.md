---
name: external_mcp_connection
description: 用户与外部 OAuth MCP 的连接状态和代理使用
dependencies:
    - media_agent_gateway/mcp_catalog
    - media_auth/oauth_credential
related_workflow:
    - external_mcp_oauth_proxy
---

## 一句话职责

External MCP Connection 负责用户到已批准外部 OAuth MCP 的连接状态，并在调用时向 Auth 获取可用凭据完成代理。

## 边界

### 做什么
- 发起连接、处理 callback、断开并查询连接状态。
- 仅代理已批准的 External OAuth MCP。
- 调用 Auth 获取或刷新 OAuth Credential。

### 不做什么
- 不持久化 OAuth access/refresh token 明文。
- 不代理 Platform MCP、内部业务 MCP或无认证外部 MCP。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 建立连接 | 发起 OAuth 并登记连接 | `BeginAuthorization/Callback` |
| 查询连接 | 返回用户连接状态 | `GetConnectionStatus` |
| 断开连接 | 撤销 Auth Credential 引用 | `Disconnect` |
| 代理调用 | 带 Auth 提供的 Token 调外部 MCP | `ExternalMcpOAuthProxy` |

## 数据模型

### `external_mcp_connections`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | External MCP Connection ID |
| user_id | text | 建立连接的用户 ID |
| definition_id | text | 外部 OAuth MCP Definition ID |
| auth_credential_id | text | media_auth 中的 OAuth Credential ID |
| status | text | pending/connected/reauth_required/disconnected |
| granted_scopes | text[] | 外部授权范围 |
| connected_at | timestamptz nullable | 最近连接成功时间 |
| last_error_code | text nullable | 最近稳定错误码 |
| updated_at | timestamptz | 最后状态更新时间 |

### UUID 生成规则

Connection 使用 ULID，OAuth Credential ID 由 Auth 生成并仅作跨服务引用。

## 能力详细说明

### ExternalMcpOAuthProxy

1. 用 Session Token 解析 user/workspace/session，并确认 Definition 已批准。
2. 加载用户 Connection，向 Auth 按 connection/purpose 获取可用 access token。
3. Token 仅存在请求内存并脱敏；外部响应直接回传，不保存为 Connection 状态。
