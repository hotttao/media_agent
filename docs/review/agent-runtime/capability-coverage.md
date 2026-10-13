# 003 Agent 架构能力覆盖与 Harness 映射

日期：2026-10-12。状态列表示目标设计覆盖，代码完成情况由落地 plan 单独追踪；covered 不等价于已实现。

事实来源：Gateway 的 chat/agent_task/batch_run/teamwork_runtime/agent_api_proxy IDL、main.go、RunTurn 与 Session Repository；Agent runtime_session/runtime_workspace OpenAPI、ACP routes、ExecuteTurnFlow、配置 Hooks 与 Alembic；Control Workspace、Prompt、Skill、Memory、Runtime Environment 的 IDL/service/migrations。media_plugin 目前只有 DDD，没有服务目录，Plugin 具体实现与 OAuth 服务不迁入 Gateway。

| 原能力/入口 | 目标上下文 | Domain/聚合或读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|
| Chat Create/List/Default Session | Gateway | Agent Session Registration | agent_session_startup | covered |
| RunTurn、Agent Task、Batch Run、Schedule、Teamwork 提交 | Agent（接入仍归 Gateway） | Runtime Session/持久 FIFO | execute_turn、现有 dispatch | covered |
| Interrupt、Agent Request list/respond | Agent | Turn/Agent Request | run_turn、execute_turn | covered |
| Message/Event/Chat Status 查询及 SSE | Agent/Gateway | Agent 事实；Gateway 本机接收者 | run_turn | covered |
| Redis Pub/Sub 的实时通知 | Agent/Gateway | Session Event Bus port | RabbitMQ 实例队列 + 本机广播 | covered |
| ACP /chat、/reconnect 的直接公网接入 | Agent | Runtime adapter | 无 | deprecated-with-reason：客户端统一 Gateway，ACP 原生执行能力保留在内部 |
| Agent /health、/info | Agent | 运行健康与 Runtime 信息 | 无 | covered |
| /reload-config、凭据/Skill 加载 | Agent | Runtime Metadata Application | agent_metadata_update | covered |
| /local/reset、Runtime Workspace 查询/重建 | Agent/Control/Gateway | Workspace Directory | workspace_lifecycle | covered |
| 启动 running Turn 恢复、后台队列排空 | Agent | Runtime Session/执行所有权 | execute_turn、session_replica_routing | covered |
| Gateway 通用 Agent API 代理、Token 签发 | Gateway/Auth | 委托身份、API Token | agent_api_proxy、agent_api_token_provisioning | covered |
| MCP 服务目录 CRUD、tools/prompts/resources 发现与调用 | Gateway | MCP Definition；请求作用域协议/分页读模型 | platform_mcp_invocation | covered |
| Workspace Plugin Binding 的启用与 OAuth Connection | media_plugin/media_auth | Binding/Connection/Credential | plugin_binding_snapshot、external_mcp_oauth_proxy | external-owner：media_plugin + media_auth，Gateway 消费其契约，不复制业务事实 |
| Platform Prompt render/write | Gateway/Agent | 模板；版本产物 | platform_prompt_assembly | covered |
| 用户 Configured Prompt 与 Skill/Memory CRUD | Control | Agent Catalog | 当前 CRUD；明确选择才进入更新流程 | covered |
| 用户确认更新、完整快照、更新通知与结果汇总 | Control/Agent | Metadata Release/实例应用投影 | agent_metadata_update | covered |
| 环境授权、Placement、目录引用 | Control | Runtime Environment | workspace_lifecycle | covered |
| Kubernetes CR 提交/资源读取；创建 StatefulSet/Headless/PVC | Gateway/自定义控制器 | Agent Instance；K8s 原生资源 | agent_workload_provisioning | covered |
| Kubernetes 扩缩容、Session 重绑、drain 后继续 queued | Gateway/Agent/StatefulSet | 绑定、Turn Queue | session_replica_routing | covered |
| Docker 单容器创建与持久目录 | Gateway/Docker 创建服务 | 创建幂等记录 | agent_workload_provisioning | covered |
| Docker 多副本/扩缩容 | 环境实现 | 无 | 无 | deprecated-with-reason：用户明确本期不考虑 Docker 扩缩容 |

## P0 立项时发现的实现缺口

以下保留为设计立项的基线问题。P1/P2/P3/P4 的解决进展以阶段审查和落地 plan 为准，不能将这份设计覆盖矩阵当作当前代码缺口清单。

- RunTurn.Start 把浏览器 authToken 填入 SessionToken，尚未实现独立稳定的 Session Credential。目标接口必须分离浏览器凭据、Agent API Token 与 Session Token。
- queued Turn 的 write-only 凭据保留在 ExecuteTurnFlow 内存；接管需根据 session/context 引用经 Gateway 重取，不迁移秘密。
- 启动恢复扫描全库 running Turn；必须按 Workspace 和执行实例/代次限定，旧代次晚到写入需执行所有权校验。
- Agent Hooks 仍使用旧 /_cp 路径并在进程内渲染平台模板；需要新的 Gateway 配置查询契约。
- Agent Catalog 只保存当前可变行，尚无不可变资源/配置发布版本；不能用当前行冒充用户曾经选定的快照。
- Gateway 只有按 Runtime 类型的静态地址，没有 Workspace/replica 路由，MCP 目录/统一协议入口与资源创建未实现。

上述为工程落地缺口，不删除或缩减目标能力。未实现的外部 Plugin 服务通过受控 HTTP port 对接，测试使用真实协议服务器验证 Gateway 行为。

## Harness 映射

| 能力 | 稳定规则/接口 | 应用编排 | 存储/外部适配 | 接入 |
|:---|:---|:---|:---|:---|
| Session 路由 | Gateway domain/agent_session、agent_instance | workflow/session_replica_routing | dal/repo + shared/client | IDL 生成 handler |
| MCP | Gateway domain/mcp_catalog | workflow/platform_mcp_invocation | dal/repo + shared/client | /mcp 协议 handler，IDL 定义路由 |
| 元数据发布 | Control domain/agent_configuration | workflow/metadata_update | dal/repo + Broker adapter | Control metadata IDL |
| 元数据应用 | Agent domain/runtime_metadata | workflow/metadata_update_flow | clients、版本产物文件适配 | FastAPI schema/IDL |
| Turn 执行 | Agent domain/runtime_session | execute_turn_flow | DAL 与 Runtime adapter | runtime_session router |
| 资源创建 | Gateway 接口与创建规格 | workflow/agent_workload_provisioning | Kubernetes 客户端/Docker 创建客户端 | workload IDL |
| RabbitMQ | Session Event Bus/本机 Source | 启动与通知消费者 | 基础设施 AMQP adapter | SSE handler 只使用本机订阅 |

检查：Go gofmt/go test/arch-check；Python ruff/mypy/lint-imports/vulture/pytest。生成文件只由 hz/protoc 更新。
