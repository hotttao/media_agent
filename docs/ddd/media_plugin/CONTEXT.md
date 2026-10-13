# media_plugin 扩展实现上下文

管理 Workspace 启用关系和外部 OAuth MCP 连接，提供具体 Plugin 能力；MCP 服务注册目录归 Gateway。具体 MCP 实现提供工具能力，Agent 统一经 MCP Gateway 访问。

## 业务术语词典

**Workspace Plugin Binding（Workspace Plugin 绑定）**:
Workspace 对一个已批准 MCP Definition 的启用状态和非敏感配置。
_禁用替代词_：ACL、Installation、Deployment

**MCP Configuration Snapshot（MCP 配置快照）**:
Gateway 根据 Workspace Plugin Binding 构建并下发给 Agent Runtime 的版本化 MCP 配置集合。
_禁用替代词_：Plugin Binding、Runtime Context

**External MCP Connection（外部 MCP 连接）**:
某用户与一个已批准外部 OAuth MCP 的连接状态，引用 `media_auth` 持有的 OAuth Credential。
_禁用替代词_：MCP Definition、MCP Token

**Internal Business MCP（平台内业务 MCP）**:
随平台业务服务部署、由 MCP Gateway 代理访问的 MCP；在经过验证的用户和 Workspace 上下文中执行工具能力。
_禁用替代词_：Platform MCP、External MCP

**External OAuth MCP（外部 OAuth MCP）**:
位于平台信任域外、需要用户 OAuth 凭据的 MCP，仅此类型经 `media_plugin` 代理。
_禁用替代词_：Internal Business MCP、Unauthenticated MCP
