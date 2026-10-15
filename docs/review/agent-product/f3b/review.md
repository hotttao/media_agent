# F3b · Workspace MCP 配置与实际调用

## 完成范围

新增 media_plugin 服务，提供 Workspace 绑定创建、CAS 编辑、列表、详情、不可变历史及受控固定版本查询。每个绑定固定 Definition 版本和能力白名单，静态凭据通过独立记录加密保存；API 仅返回配置标志，历史与 Release 不包含明文。替换凭据形成新候选引用，旧会话保留原选择；当前停用则阻止后续调用。

Control 明确确认接口接受 media_plugin.mcp_binding ID/version，经 Plugin 校验真实 Workspace、企业目录及当前权限后保存非秘密引用。编辑绑定不会发布 Agent 通知，只有明确确认发布 Release。Plugin 运行查询从 Gateway 已验证的 Session 固定 Release 解析选择，不读取最新候选代替。

无需认证、Bearer、API Key 通过 Gateway 受控连接；OAuth 经 Plugin 代理，Auth 新增专用服务身份的可用 token 端口，复用现有授权和刷新流程，不返回 refresh token。缺少连接返回 needs_reauth；临时刷新故障保留连接并返回服务不可用。OAuth SQL 禁止输出带凭据的参数。共享官方 MCP 上游适配和 Hertz 协议桥接移入 hertz_infra，保留禁止重定向、跨源 SSE 凭据外发及写重试的行为。

Plugin 提供启动配置、独立数据库迁移、OpenAPI、Dockerfile 和接入说明。本地 platform 内置 MCP 无需 Plugin Binding；外部绑定管理支持四种认证类型，平台 session_context 仍保留在 Gateway 的可信平台实现边界。

## 验证

- Gateway、Control、Auth、Plugin 全模块 Go 测试及共享协议包通过（/tmp/agent-product-f3b-all-checks.txt）。后续凭据错误区分、Header 限制和受控客户端修改另经相关测试验证（/tmp/agent-product-f3b-token-final.txt、/tmp/agent-product-f3b-final-checks.txt）。
- 四个服务 archcheck 均通过；git diff --check 通过。
- 独立 PostgreSQL 实际执行四个服务迁移，真实 HTTP + 官方 MCP SDK 联调最终通过，64.393s（/tmp/agent-product-f3b-final-integration.txt）。Auth 企业/部门和入口 Agent 身份是明确 fixture，目录、绑定、配置 Release、凭据及 OAuth token 使用实际服务和数据库。
- 联调覆盖四种外部认证的工具发现与调用、OAuth 过期凭据真实 HTTP 刷新、权限拒绝、内部端口凭据隔离、保留 Header 拒绝、重复创建/更新冲突、失败事务不遗留凭据、公开响应与历史不泄露秘密、编辑不产生通知、旧会话保留旧凭据、新会话明确切换、停用立即撤销、禁用引用确认拒绝、OAuth 断开与重新连接入口。
- 已编译并实际启动 Plugin 二进制，验证配置加载、迁移、HTTP ping 与私有接口拒绝匿名访问；随后终止进程并删除专用 smoke 数据库（/tmp/agent-product-f3b-process.log）。
- IDL 通过工具重新生成模型、客户端和路由；Auth 与 Plugin OpenAPI 同步。

## 后续

F4 完成可操作界面、真实 Edge 路由和 Docker 正式 Agent 使用验收。当前联调不代表真实模型或浏览器完整产品已验收；没有启动 k3s，没有新增扩缩容、隔离或 SRE 专项。
