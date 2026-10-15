---
name: external_mcp_connection
description: 已批准外部 OAuth MCP 的当前用户连接读模型与代理准入
dependencies:
    - media_agent_gateway/mcp_catalog
    - media_auth/mcp_oauth
related_workflow:
    - external_mcp_oauth_proxy
---

## 一句话职责

External MCP Connection 将已批准 Definition 和 Auth 持有的用户 OAuth 连接组合为可连接、已连接或需要重新授权的当前用户读模型。

## 边界

- Plugin 校验 Workspace 与固定 Binding，提供批准来源的 OAuth 入口及连接状态，代理外部 OAuth MCP。
- Auth 负责授权发起、callback、连接持久化、断开、OAuth token 及刷新。Plugin 不保存第二份 OAuth 连接事实或 token。
- Platform MCP、内部业务 MCP及无需认证外部 MCP不经过 OAuth Proxy。

## 能力清单

| 能力 | 事实所有者与入口 |
| --- | --- |
| 建立连接 | Plugin Binding 详情返回批准来源的 Auth authorize URL；Auth 处理浏览器授权与 callback |
| 查询连接 | Binding 详情根据 Auth 的当前可用凭据返回 connected/needs_reauth |
| 断开连接 | Auth 既有 disconnect API，Plugin 下次调用读取当前事实 |
| 代理调用 | Plugin 核验固定 Control Release 与当前准入后使用 Auth access token |

## 模型与不变量

这是跨服务查询形成的读模型，不是新的持久化聚合。连接以已验证 user_id 与固定 Definition endpoint 的规范化 server_origin 查询 Auth；Definition ID 保留用于界面选择和调用错误定位。不存在独立 `external_mcp_connections` 事实表，也不复制 Auth 的刷新状态。

Auth token 只在请求内存使用，不能出现在 Plugin 数据库、Binding 历史、Control Release、Agent 文件或日志中。连接状态只描述当前用户，不能由客户端指定其他用户或任意上游 URL。已断开或需要重新授权时不代理调用；Auth 故障不能解释为已连接。

## ExternalMcpOAuthProxy

Gateway 已验证 Agent/Session/执行上下文，使用专用服务凭据将固定 Release 标识及用户作用域传给 Plugin。Plugin 再核验 Workspace view、Control 固定 Release、Binding 与 Definition 当前准入，随后向 Auth 查询可用 access token，只连接固定目录 Endpoint。OAuth 授权和代理调用均不自动升级 Agent 配置。
