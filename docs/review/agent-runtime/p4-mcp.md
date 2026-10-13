# P4 · MCP Gateway 与 Platform Prompt 审查

日期：2026-10-13。范围依据 [阶段计划](../../plan/agent-runtime.md) 与 [MCP 规格](../../spec/agent-mcp-gateway.md)。阶段结束暂停；资源创建和扩缩容属于 P5。

## 已完成能力

Gateway 自有 MCP Catalog 保存受控 Definition 当前版本、不可变历史、revision 和变更 Outbox。平台 admin 角色由 Auth 当前用户查询取得；内部服务只读接口支持固定历史查询。写入以 expected_version CAS，禁止退役复活、未经批准的 host、重定向或把内置 platform 改成远端来源。独立 relay 通过 RabbitMQ publisher confirm 投递目录变更提示，稳定 event_id 允许重复；该提示不触发 Agent 配置更新。

标准 /mcp 入口使用官方 Go SDK v1.8.0，兼容 initialize 与 server/discover。Gateway 是无状态入口，上游在请求范围独立协商并关闭；同一请求的分页与调用复用协议连接。入口 SDK 元数据不进入独立上游握手，取消/deadline 保留。入口支持 tools、prompts、resources、resource templates；来源命名空间、长名称映射、资源 URI/嵌入引用、分页及固定来源版本均由 Gateway 协调。签名 cursor 绑定 Session、权限/目录/内容向量，两个 Gateway 可以继续同一分页；权限或来源变化后拒绝旧 cursor。

每次 HTTP 请求与业务 RPC 都验证独立 Session Token、稳定 Agent 委托及当前 Control 权限；托管调用再验证实例凭据、原生代次和实际 running Turn/epoch。发现后、发业务调用前再次读取权限，旧实例、queued Turn、错作用域和停用来源不能调用。后端只接收服务计算的身份与 Plugin 认证适配，入口 Token 不透传。资源最终授权仍属于后端。needs_reauth 产生用户重连错误；连接损失或响应不明不自动重试写操作。JSON 整数参数保留原精度。调用日志不含凭据、参数或结果正文；可靠采集属于部署观测设施。

Platform Prompt 模板及正文渲染迁至 Gateway，Configured Prompt 独立交付。Control release 固定模板引用；历史缺失引用继续使用版本1，用户明确确认的新 release 固定版本2，未知模板拒绝回退。Agent 删除本地模板/渲染器，仅落盘加载正文；旧 Session 的选择、文件与原生句柄保持。平台只声明已实现的目录/MCP/Skill/Memory 约定，没有虚构挂载或工具。

Codex、Claude、Goose 托管执行只配置 Gateway。ACP 按轮关闭空闲连接并用已有 native handle 加载，刷新 Turn/epoch 请求头而保留原生历史；Claude 使用 strict MCP 配置。Goose 修复原有非标准 ACP 调用，改用通用 SDK 与官方 Prompt 扩展，两个独立 key 加载平台和用户正文，按稳定 Session 隔离原生存储，并提供固定 Skill artifact 的平台只读工具。旧 CpClient 只适配 Gateway 元数据，不再调用 /_cp 或写入后端 MCP/provider 凭据。

## 验证证据

| 检查 | 结果 |
|:---|:---|
| Gateway、Control Go 全模块测试与 arch-check | 通过 |
| MCP 真协议服务器 | 标准 SDK：上游多页发现、两个来源重名、两个 Gateway 继续分页、Prompt/资源/模板读、模板权限匹配、停用/撤权、参数精度 |
| 兼容与上游 transport | 2025-03-26、2025-11-25、2026-07-28；Streamable HTTP/SSE；缺失能力返回空列表；private/ttl=0/no-store；OAuth 重连错误、禁止重定向/SSE 跨源消息地址、真实写连接断开只发送一次 |
| Hertz 与并发 | 协议状态/header/body 保持；发现/调用并发 race 检查通过 |
| PostgreSQL Catalog | 当前/历史/Outbox 同事务、并发 CAS 单一赢家、退役不复活；真实 RabbitMQ 确认后标记投递 |
| 执行身份链路 | 原 P3 Hertz + 两个 Python HTTP 实例 + PostgreSQL/RabbitMQ，追加 MCP MAC/owner/incarnation/epoch/running Turn 验证，接管后原身份拒绝 |
| 元数据链路 | 原 P2 Control/Gateway + Python + PostgreSQL/RabbitMQ 通过，模板版本引用与固定选择兼容 |
| Python 全量 | 默认 pytest 已覆盖 adapter/codex/goose/claude_code/tests：115 passed，无跳过；仅既有 Starlette deprecation warning |
| Runtime 协议 fixture | Codex/Goose 真实 ACP SDK 子进程 new/load 保留句柄并刷新 Turn 请求头；Goose Prompt 扩展按两个独立 key 保持固定正文；Claude 子进程 strict 配置仅含 Gateway |
| Python lint/格式/跨包层级 | 通过 |
| IDL/Swagger | hz/protoc 生成管理/内部目录/API 客户端与路由，配置 protobuf 和 Swagger 同步 |

Python 测试数量从 P3 的119变为115：旧 Agent 渲染器与 /_cp 合并测试迁移为 Gateway 正文和受控接口契约测试，另增加两个 ACP Runtime 集成场景。没有跳过真实数据库/Broker 测试来降低数量。

## 阶段边界与保留门禁

Plugin/Auth 的 Binding、OAuth 和固定 context_version 到快照映射属于外部服务，仓库当前无 media_plugin 服务实现。此阶段交付受控 IDL/生成客户端及代理行为，使用真实协议服务器与授权端口 fixture 验证，没有伪造外部服务部署。无 Plugin URL 时明确只提供内置 platform；配置外部 Plugin 但其不可用时拒绝，不取最新配置或回退。

入口不保留跨请求上游会话，不提供后台订阅、listChanged、sampling 或后台任务。未来扩展后台连接需设计协议引用持久化；当前 scope 不引入无用途的上游引用表。Runtime fixture 验证真实 SDK/子进程契约，未冒充真实模型调用或原生 Goose/Codex/Claude 的端到端生产运行。

Python 全量 mypy 从56条既有问题降为28条，归一化比较无新增；import-linter 仍13 kept/1 broken（既有 ACP Domain 对 shared.clock/id 的依赖）；vulture 当前109项，新增的 AcpSessionHandler 是保留旧 typing import 的兼容别名。保留问题继续在 P6 处理，不宣称全部项目门禁通过。

默认 AGENT_MCP_ENABLED=false；启用需要 P3 托管身份配置、持久密钥、受控 host 与外部 Plugin 凭证。没有修改部署、运行生产迁移或创建提交。阶段测试只使用独立 PostgreSQL/RabbitMQ，结束后移除。
