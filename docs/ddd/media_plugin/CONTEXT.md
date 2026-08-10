# media_plugin 扩展目录上下文

管理平台受控发布的 MCP 定义、Workspace 启用关系和外部 OAuth MCP 连接。它不接收用户上传代码，也不转发平台内业务 MCP。

## 业务术语词典

**MCP Definition（MCP 定义）**:
由平台运维登记的 MCP 服务描述，包含稳定标识、服务身份、Endpoint 和认证类型。
_禁用替代词_：Plugin Instance、MCP Connection、Tool List

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
随平台业务服务部署并由 Agent 直接调用的 MCP；通过 Session Token 获得用户和 Workspace 上下文。
_禁用替代词_：Platform MCP、External MCP

**External OAuth MCP（外部 OAuth MCP）**:
位于平台信任域外、需要用户 OAuth 凭据的 MCP，仅此类型经 `media_plugin` 代理。
_禁用替代词_：Internal Business MCP、Unauthenticated MCP
