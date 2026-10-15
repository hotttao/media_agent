---
name: external_mcp_oauth_proxy
description: 基于固定 Workspace Binding 和当前 Auth 连接代理外部 OAuth MCP
participants: [media_agent_gateway, plugin_binding, media_control, media_auth, external_mcp]
domains: [plugin_binding, external_mcp_connection]
---

## 触发与结果

用户从 Binding 详情打开 Auth 既有 OAuth authorize，Auth 完成 callback 并持有连接。Agent 调用时只连接 Gateway；Plugin 在固定配置和当前准入均允许后使用 Auth 提供的可用 access token 代理外部 MCP。缺少连接返回 needs_reauth；服务故障或调用结果未知不得解释为成功。

## 执行时间线

```mermaid
sequenceDiagram
    participant U as User
    participant A as Auth
    participant R as Agent
    participant G as Gateway
    participant P as Plugin
    participant C as Control
    participant E as External MCP
    U->>P: 查询已批准 Binding 详情
    P->>A: 专用凭证查询用户与固定 server_origin 的连接
    P-->>U: connected / needs_reauth 与 Auth authorize URL
    U->>A: OAuth authorize
    A->>E: 动态注册、授权与 code exchange
    E-->>A: access/refresh token
    A->>A: 保存用户连接
    R->>G: MCP request + Agent/Session/执行身份
    G->>G: 验证身份、真实企业与工具权限
    G->>P: 专用 Gateway 身份 + 固定 Release/Binding + 验证后作用域
    P->>C: Workspace view 与该 Session 固定 Release
    C-->>P: 权威作用域及固定 Binding 引用
    P->>G: 查询当前与固定 Definition
    G-->>P: 已批准地址、企业、状态和版本
    P->>P: 校验当前 Binding 准入与固定摘要
    P->>A: 专用 Plugin 凭证 + user/server_origin
    A->>A: 读取有效 access token，必要时执行既有刷新
    A-->>P: access token / needs_reauth / 故障
    P->>E: 独立协商 MCP 协议并调用固定 Endpoint
    E-->>P: MCP result / 错误 / 结果不明
    P-->>G: MCP result 或认证/调用错误
    G-->>R: 统一命名空间中的 MCP result
```

## 一致性与生命周期

Auth 连接保存和刷新属于 Auth 本地事务；Plugin 不写连接副本，读取凭据与外部调用不进入 Binding 事务。用户关闭弹窗不取消已经完成的 Auth 保存。Gateway/Plugin 每次调用读取当前准入，Binding/Definition 停用或用户断开连接立即影响后续请求，不主动中断已经执行的外部工具。

Binding 编辑及 OAuth 连接不发布 Agent 配置通知。Session 固定 Control Release 不随最新 Binding 改变；OAuth 当前用户连接独立于固定配置，断开后需要重新授权。

## 权限、失败与重试

Agent 的 Token 不用于外部 OAuth；插件代理只接受专用 Gateway 凭证，作用域来自 Gateway 验证结果，不能从工具参数覆盖。Auth 可用凭据端口只接受专用 Plugin 凭证，响应 no-store，不返回 refresh token。Plugin 只使用目录固定 Endpoint；HTTP 重定向与 SSE 跨源 message Endpoint 不可携带凭据。

无连接时 Plugin 返回401 needs_reauth，Gateway 将其映射为可重连的 MCP 错误。Auth/Control/目录不可用时拒绝，不使用旧授权缓存。外部调用超时或响应丢失不自动重发，写工具结果未知时由用户核实。刷新行为沿用 Auth 既有服务，不新建后台连接同步、重试队列或单航班协调器。
