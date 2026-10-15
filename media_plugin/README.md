# media_plugin

Workspace MCP 绑定与外部 OAuth MCP 代理。目录归 Gateway，期望配置 Release 归 Control，OAuth 用户凭据归 Auth。

启动 `go run .`；先为本服务创建独立 PostgreSQL 数据库，通过 POSTGRES_DSN 配置，启动自动执行 migrations。Docker 在仓库根目录执行 `docker build -f media_plugin/Dockerfile -t media-plugin .`。

| 环境变量 | 用途 |
| --- | --- |
| SERVER_ADDR | 默认 :8011 |
| POSTGRES_DSN | Plugin 独立 PostgreSQL 数据库 |
| MEDIA_CONTROL_URL | Control HTTP 地址 |
| MEDIA_AGENT_GATEWAY_URL | Gateway HTTP 地址 |
| MEDIA_AUTH_URL | Auth HTTP 地址 |
| CONTROL_INTERNAL_TOKEN | Control / Gateway 内部读请求凭证 |
| MCP_CONTROL_CREDENTIAL | Control 查询固定 Binding 的专用凭证，两端相同 |
| MCP_GATEWAY_CREDENTIAL | Gateway 查询调用权限及访问 OAuth 代理的专用凭证，两端相同 |
| AGENT_METADATA_GATEWAY_CREDENTIAL | Control 固定 Release 查询凭证，与 Control metadata 配置相同 |
| MEDIA_AUTH_MCP_PLUGIN_TOKEN | Auth 可用 OAuth token 端口专用凭证，两端相同 |
| MCP_CREDENTIAL_ENCRYPTION_KEY | 非空静态凭据加密密钥，替换前需迁移旧加密数据 |

Control 增加 MEDIA_PLUGIN_URL、MCP_CONTROL_CREDENTIAL；Gateway 设置既有 agent_mcp.plugin_url 和 plugin_credential；Auth 设置 MEDIA_AUTH_MCP_PLUGIN_TOKEN。浏览器经 Edge 路由 /api/v1/workspaces/{id}/mcp-bindings 到本服务，OAuth 浏览器授权仍路由至 Auth /api/mcp-oauth。内部端口不公开给浏览器。

绑定默认选用全部工具/Prompt/Resource，可配置显式白名单。POST 创建与 PUT expected_version 更新产生不可变历史；未提供 credential 保留，空字符串清除当前候选引用，非空替换为新的加密凭据引用。API Key 支持 credential_header，Bearer 使用 Authorization。响应与历史不返回密钥。

编辑仅产生候选 Binding 版本。用户确认时通过 Control resources 选择 media_plugin.mcp_binding 的 ID/version；运行只读取 Session 固定的 Release。旧会话保留旧版本与对应静态凭据；停用当前 Binding/Definition 立即阻止后续调用。OAuth 断开连接立即需要重新授权，Plugin 不保存 OAuth token。

检查：`go test ./...`、`make arch-check`。跨服务真实数据库测试见 harness/tools/agent_product_integration。
