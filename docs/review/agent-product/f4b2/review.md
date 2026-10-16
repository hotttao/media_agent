# F4b2 · 真实模型与完整使用验收

## 结果

本机 Docker 入口 `http://192.168.2.41:18080/agent/` 已完成真实登录、资源配置、创建 Agent 与正式 Codex 对话验收。真实模型通过固定 Skill 输出验收标记，经 Gateway → Plugin → 外部 MCP 调用返回 `mcp-e2e-ok`；人工“允许一次”后继续执行。第二轮独立回复、实时 `item.delta`、完成事件和刷新后的历史恢复通过。k3s 保持关闭。

真实浏览器使用生产界面及真实服务，没有 API 拦截；每次发送不同标记，避免旧历史满足断言。捕获32个带序号的事件，包括 item.delta，页面脚本错误为0。历史中有2个已解决的权限请求、无 pending。最终流式复测无需再次审批，因此证据中的 approvedRequests=0；resolvedPermissionRequests=2 来自同一真实 Session 的请求查询。凭据清理并重建容器后仍可查询13条历史消息。

## 修复与设计一致性

| 真实使用发现的问题 | 实现与结果 |
| --- | --- |
| ACP 恢复历史被拼入下一轮回复 | 每轮 prompt 前创建新的事件翻译器，隔离历史回放、工具状态与新输出；回归用例覆盖旧内容不进入新完成项。 |
| Codex OAuth 自动加载账号 Apps/Plugin | 固定元数据原生配置关闭这些自动扩展及 Skill MCP 自动安装；保留固定本地 Skill 与 Gateway MCP。 |
| Agent 拒绝已确认的 MCP binding 资源类型 | 元数据 schema 接受 media_plugin.mcp_binding 的固定引用；未知必需资源继续拒绝，不向 Agent 复制上游凭据。 |
| 权限请求 HTTP JSON 字符串被当作对象使用 | chat-client 统一解码 payload_json/response_json；列表、resolve 使用相同契约，非法对象不授权。 |
| 原生权限标题为空，用户无法判断待批命令 | 支持原生工具标题，单独展示待批准 command/argv；其他原始参数及环境不展开。文案支持中英翻译。 |
| Gateway/Control 与 Agent 使用不同 Broker | Compose 支持 product.env 显式覆盖旧 host Broker 默认值；三者统一 product-broker 后，真实浏览器收到实时 delta。 |

DDD、执行流程与 UI 请求契约同步更新；Control 仍持有配置事实，Gateway 仍为唯一 MCP 入口，Skill 版本化由系统控制。未增加隔离、扩缩容或运维治理功能。

## 验证

- 产品后端集成测试通过（82.231秒）：真实 PostgreSQL、HTTP、官方 MCP SDK；覆盖 Prompt ACL、Skill 导入/文件/版本、配置明确确认、初始事务回滚、MCP 无认证/API Key/Bearer/OAuth 代理。
- Python 相关回归19项通过；另有前一组22项通过。修改的6个源/测试文件 mypy 通过，Ruff 通过。
- UI 14文件34项测试通过，TypeScript 无错误；依赖边界、翻译键、修改文件 Biome 和 diff 空白检查通过。生产镜像构建及运行成功。
- 生产构建预览的浏览器回归5项通过：Prompt 编辑/历史/分享/窄屏、Skill 发布下载、MCP 写入式凭据与固定旧会话、首次启动防重复、原生权限 HTTP 请求渲染及解决。
- 正式模型验收：真实 Codex OAuth、Skill 加载、真实 MCP echo、权限确认、多轮回复、SSE delta、刷新历史均通过。见 [结构化证据](live-acceptance.json) 与 [实际界面](conversation.png)。

## 代理与验收清理

没有为 Agent 设置 ALL_PROXY。临时配置 HTTP_PROXY/HTTPS_PROXY，经 Docker 可达的宿主内网转发到本机127.0.0.1:7897，并对内部服务配置 NO_PROXY。原生 Codex 曾报告 WebSocket TLS EOF，自动回退 HTTPS 后成功回复；这说明模型通路可用，不能把单次 curl/TLS 探测失败解释为整个代理不可用。截图保留这一原生回退提示，没有隐藏运行结果。

按用户授权，登录仅复制到专用验收 Workspace。停止原生进程后删除4份 OAuth auth.json 副本，剩余0；本机原始登录未修改。已停临时代理转发与 MCP fixture、禁用测试 MCP 定义/绑定，并明确确认移除临时 MCP 的新配置版本5；旧 Session 仍固定版本4。移除临时允许地址和代理设置，恢复 Controller 默认创建策略；专用 PostgreSQL 测试容器已删除。产品数据库、服务、历史和正常部署密钥保留。

## 验收边界

正式模型验收覆盖 Codex；Claude/Goose 沿用既有适配器检查，本次未使用其正式模型凭据验收。真实外部 MCP 使用无认证的官方 SDK 服务；其余认证模式由真实协议/HTTP 集成测试验证，未代表连接任意生产 OAuth 提供商。模型登录副本已清理，继续发起模型对话需要用户自己的 Workspace 模型凭据及容器可达的出网配置，准备命令不会自动复制开发者登录。部署说明见 [AGENT_PRODUCT.md](../../../../docker-compose/AGENT_PRODUCT.md)。
