# 003 Plugin 与 MCP 规范

## 1. Plugin 形态

Plugin 是随平台发布的受控扩展，不是用户上传系统。以 `media_workflow` 为例：

- `ui_workflow` 编译后放入 `ui_platform` 的既定扩展目录。
- 后端服务与 MCP Adapter 默认同进程部署。
- 平台通过声明式目录登记服务地址、MCP 能力和启用范围。
- 普通用户不能上传前端、后端代码或任意 MCP URL。

最小登记信息示例：

```yaml
id: workflow
service: media_workflow
internal_endpoint: http://media-workflow:8002
mcp_endpoint: /mcp
```

## 2. 注册与调用

1. 运维发布 Plugin，并在 `media_plugin` 登记定义。
2. Workspace 管理员启用 Plugin；`media_plugin` 保存启用关系。
3. `media_agent_gateway` 在 Agent 启动或配置变更时获取 MCP 快照并下发给 `media_agents`。
4. Agent 按快照直接调用目标 MCP，不经过 `media_plugin` 转发。

启用关系只控制能力发现和配置下发。目标服务仍必须认证调用者并执行资源授权。

## 3. MCP 类型

| 类型 | 调用路径 | 认证 |
|---|---|---|
| Platform MCP | Agent → `media_agent_gateway` | `session_token` |
| 平台内第三方 MCP | Agent → 业务服务 MCP | `session_token`，服务解析用户与 Workspace |
| 外部无认证 MCP | Agent → 外部 MCP | 无平台凭据 |
| 外部 OAuth MCP | Agent → `media_plugin` → 外部 MCP | `session_token` + `media_plugin` 托管的 OAuth 凭据 |

Platform MCP 属于 `media_agent_gateway`。平台内第三方 MCP 与业务服务同进程，不通过 Plugin 代理；外部 OAuth MCP 才由 `media_plugin` 完成凭据选择、刷新和转发。

## 4. 变更规则

- 新增业务 Plugin 不修改 `media_control`。
- MCP 配置变更生成新快照并通知运行中的 Agent；是否热更新由 Agent Runtime 决定。
- MCP 响应中的大文件只返回业务资源标识或 Storage 路径，不经 Gateway 转发字节流。
