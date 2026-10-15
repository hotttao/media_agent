# P4 · MCP Gateway 与独立 Platform Prompt

依据 agent-runtime.md。Gateway 拥有 MCP Catalog 与平台模板，Plugin/Auth 拥有 Binding/外部 OAuth，Control 继续拥有固定配置。所有网络操作在数据库事务外。

## 受控目录

目录 API 由 Edge 签名用户身份经 Auth GetMe 取得当前平台角色，企业管理另向 Auth 查询当前企业 admin 事实，不信任用户请求中的角色。稳定 slug 限制为安全字符；Endpoint 只能使用部署允许的 host，禁止凭据、fragment、重定向和任意上游 URL。支持 streamable_http/sse 与本地 platform Transport；auth 为 session_context/oauth/none/bearer/api_key。本地 platform 由 Gateway 实现，不访问网络。更新使用 expected_version，retired 不复活，保留不可变定义历史。目录事务同时递增 revision、记录可重试变更事件；通知不自动修改用户已确认配置。

## 每次请求身份与权限

Session Token 解析 Registration，Control 按固定 context_version 重新校验 Workspace 权限。稳定 Agent API Token 的 configuration/owner/workspace 必须匹配。托管 Session 还必须携带 instance credential、replica/incarnation、execution_epoch 和 turn_id，通过 P3 当前原生代次与实际 running Turn 验证；旧实例不能利用仍稳定的 Session Token 调用工具。

Plugin 受控端口输入为已验证 user/workspace/config/context_version；返回固定快照版本、定义 ID/版本、当前获准能力与后端认证适配。没有该固定版本映射时拒绝，不能偷偷取 latest。Gateway 不保存 Binding/OAuth 或 Control 配置，也不把入口 Token 转发后端。外部 OAuth 由 Plugin/Auth 代理，needs_reauth 为明确失败，不能自动登录或重放写调用。

## 标准协议入口与映射

POST/GET/DELETE /mcp 使用官方 Go SDK 的 Streamable HTTP；Gateway 使用 stateless 请求作用域适配多实例。保留现有 Runtime 的 initialize 协议兼容；不把协议连接或 Gateway 内存状态作为授权来源。每个请求重新取得目录、固定 Binding 和当前权限。

对外工具/Prompt 名称使用稳定 Definition slug 与原名称的可逆或显式来源映射，命名冲突拒绝覆盖。资源 URI 和模板使用 mcp-gateway://<slug>/<原 URI> 前缀，去前缀即可恢复原 URI，保留模板表达式和原百分号编码。工具结果内资源引用同样按来源改写。

工具、Prompt、资源和模板发现遍历上游分页并限制总页数/大小；Gateway cursor 绑定 Session、目录/Binding/发现内容版本和位置，两个 Gateway 可以继续同一分页。版本变化返回失效 cursor，不能混合快照。调用重新授权并校验目标来源，后端执行最终资源授权；不自动重试结果未知的写操作。后端不支持的功能不宣告。

上游协议连接按受控请求作用域创建并释放，不跨进程缓存授权；不保留跨请求后台会话，不建立上游协议引用表；扩展订阅时再设计引用持久化，不保存认证秘密。SDK transport 禁止写重试和重定向。取消、网络或半响应失败归为结果不明；不能自行重放工具。

## Platform Prompt

Gateway 生成正文，Configured Prompt 分开交付。模板 immutable/versioned；Control release 固定模板版本引用，旧 release 缺失引用按 P2 版本1解释；完整新模板为版本2，只有用户确认的新 release 选择版本2。旧 Session 在不同 Gateway/Agent 上恢复时仍按原模板与同版本输入渲染，不缓存 Control 配置。

模板覆盖持久目录、统一 MCP、Skill 与 Memory 约定；不声明未实现挂载/工具，不含秘密。Agent 只转换为 Runtime 特有参数、落盘和加载，禁止保留另一套模板渲染器或下发后端地址。Codex、Claude、Goose 每次执行只配置 Gateway MCP，凭证与 owner/epoch 通过本次执行作用域注入，文件权限和 Session 目录隔离继续遵守 P2/P3。

## 验证

真实协议服务器覆盖分页、重名、资源/模板 URI、Prompt、OAuth 拒绝、目录停用/权限撤销、写超时不重试、两个 Gateway 分页与旧 Session。真实 PostgreSQL 验证目录 CAS/历史与 Session Token 解析；原生 Runtime fixture 检查只有 Gateway 配置及固定模板正文，Broker/执行回归保留。


## 具体配置与外部端口

Gateway typed agent_mcp 配置：enabled（默认false）、allowed_upstream_hosts（精确host[:port]列表）、plugin_url、plugin_credential。环境入口为 AGENT_MCP_ENABLED、AGENT_MCP_ALLOWED_UPSTREAM_HOSTS（YAML列表）、MEDIA_PLUGIN_URL、MCP_PLUGIN_GATEWAY_CREDENTIAL。开启依赖 P3 agent_runtime；无 Plugin URL 时只提供内置 platform，配置 Plugin 但不可用时拒绝而不回退。目录发布 topic exchange 为 media.mcp.catalog，事件 schema_version=1/event_type=mcp.catalog.changed/event_id/definition_id/definition_version/occurred_at。

Plugin 服务位于 media_plugin；idl/media_plugin/mcp_access.proto 定义受控查询，Gateway 使用生成客户端。请求含 user/workspace/config/session/context_version/target_context_version，返回固定 snapshot_version、当前 authorization_revision、带定义版本及 methods/tools/prompts/resources allowlist 的 grants。缺失固定映射必须拒绝。静态认证通过私有端口返回受限上游 Header，OAuth 返回固定 Binding 引用并由 Gateway 客户端根据部署配置构造 Plugin 代理地址；服务凭据与用户凭据不暴露给 Agent。none 允许无凭证受控后端，其他认证类型缺失授权适配即失败。资源权限支持精确 URI 或已批准 RFC6570 模板；目标资源所有者仍执行最终授权。

Agent MCP 请求头：Authorization（稳定 Agent Bearer）、X-Session-Token、X-Agent-Instance-Credential、X-Agent-Replica-ID、X-Agent-Incarnation-ID、X-Agent-Turn-ID、X-Agent-Execution-Epoch。Gateway 根据权威事实重新授权，不信任客户端 Workspace/owner 声明。后端仅收到 Plugin 适配认证及服务计算的 X-Media-User-ID/Workspace-ID/Session-ID/Agent-Configuration-ID。

返回 -32602 表示非法参数/失效 cursor；-32001 表示拒绝；-32002 + outcome_unknown/retryable:false 表示可能已执行；-32003 + needs_reauth/definition_id/retryable:false 表示需要用户重连。HTTP 身份拒绝401，权威服务临时失败503。调用 structured audit 不记录凭据、参数或结果正文，可靠采集由部署观测设施承担。

入口兼容2025-03-26、2025-11-25和2026-07-28；SDK v1.8.0（官方 [release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)）。Gateway 返回 Cache-Control:no-store，并强制发现/资源 private、ttl=0。请求2MiB、上游响应16MiB、发现最多64页/来源和2000条总结果、Gateway页面100条、签名cursor有效600秒、代理整体30秒；不声明 listChanged/subscribe/sampling/后台任务能力。

Codex/Goose 的托管 ACP 每轮关闭已空闲的连接，用持久 native handle load Session，并替换 Gateway 请求头；不删除历史，不重放前一轮。Claude 每轮用 --strict-mcp-config 和内联 --mcp-config，仅包含 Gateway。Goose 使用标准 ACP，按稳定业务 Session 隔离 native存储；通过官方 `_goose/unstable/session/system-prompt/set` 扩展按两个独立 key 加载固定 Platform/Configured 正文（[schema](https://github.com/aaif-goose/goose/blob/main/crates/goose/acp-schema.json)）。不支持该扩展时明确加载失败。Skill 版本由系统选择与文件/授权控制，Goose 可通过 platform__read_skill 读取 Session 已选择的固定 artifact，并校验 checksum；能力声明不能绕过固定版本。

旧 CpClient 仅保留 Gateway 元数据适配，不再调用 /_cp，也不拉取/落盘任意后端 MCP 或 provider credentials；生产托管模式使用 GatewayMetadataClient/GatewayInstanceClient。未配置受控身份的本地模式仅用于 mock，本阶段不提供旧 CP 远端兼容服务。

部署顺序：先完成所有 Gateway 的固定版本模板/协议能力部署，再允许 Control 发布模板版本2；已有 release 继续版本1。不能在旧 Gateway 仍忽略模板引用时发布新版本。滚动升级期间不以新二进制重新解释旧 Session 的模板版本。SSE 上游的消息 endpoint 也必须与已批准地址同源，不能利用 endpoint 事件把后端授权发往其他 origin。ACL 向量按集合排序去重，表示顺序变化不会使跨 Gateway cursor 失效。

企业目录及 Workspace 绑定产品契约、静态认证、OAuth 和明确确认的接口与版本细节见 [agent-product.md](agent-product.md)，该规格补充 P4 的早期基础实现。
