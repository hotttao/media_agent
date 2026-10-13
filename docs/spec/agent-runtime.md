# 003 Agent 架构实现规格

状态：核心接口已通过22个伪代码模型场景验证，真实协议/数据库/环境集成验证仍须执行。全部业务事实仍由权威上下文保存，接口不依赖共享 Gateway 内存。

## 身份、版本与错误

- Web 接口使用现有用户身份，Control 最终校验 Workspace manage/use。Agent 拉取与上报使用 Agent API Token 经 Gateway 验证稳定 configuration_id；Session MCP 使用独立 Session Token。
- runtime instance 定位键为 environment/workspace/replica_id；incarnation_id 标识本次进程启动。Gateway 校验 Kubernetes Pod UID/容器身份或 Docker 创建记录，不能单凭上报正文授权。
- context_version 固定 Session；metadata selection_revision 表示用户确认次序，可选择旧资源版本；binding_version 是 Gateway 绑定 CAS；execution_epoch 是 Agent 本库会话执行所有权。
- 400 invalid_argument；401 unauthenticated；403 permission_denied；404 not_found；409 version_conflict/stale_owner/idempotency_conflict；422 configuration_invalid；503 temporarily_unavailable。结果不明不伪造业务终态，不自动重放非幂等工具。

## Gateway 创建与实例接口

| 方法 | 路由 | 输入/结果 |
|:---|:---|:---|
| EnsureWorkload | POST /internal/agent-workloads/{workspace_id}:ensure | expected Control version + idempotency_key；返回 workload_ref/status 与稳定地址引用 |
| GetWorkload | GET /internal/agent-workloads/{workspace_id} | Kubernetes 原生资源读取或 Docker 创建结果，不调用 Controller 探活 |
| ScaleWorkload | POST /internal/agent-workloads/{workspace_id}:scale | expected revision + replicas；K8s 缩容先 drain，再提交 CR 副本目标；Docker 拒绝 scale |
| ResolveInstanceIdentity | POST /api/agent/v1/instances/identity | 稳定 Agent 身份、原生身份凭证及定位；返回已核验资源 UID/generation/type 与登记版本 |
| RegisterInstance | POST /api/agent/v1/instances/register | 稳定 Agent 身份、replica/环境身份、incarnation；返回实例凭证、登记版本、准入状态/排空操作，不缓存配置 |
| GetRuntimeContext | GET /api/agent/v1/runtime-context | selection_revision + target_context_version，或当前已确认目标；返回 Control 快照 + 独立 Platform Prompt |
| ReportMetadataResult | POST /api/agent/v1/metadata-results | report_id、selection/target、replica/incarnation、状态/错误码；代理 Control 幂等写投影 |
| GetExecutionCredentials | POST /api/agent/v1/sessions/{session_id}/execution-context | 已验证实例 + execution_epoch + turn_id；返回 write-only 执行凭据；不把浏览器 Cookie 当 Session Token |

Ensure/Get/Scale 仅受信 Control 服务及已授权用户可调用；注册和配置查询不接受任意 URL。Gateway 不缓存 Control 配置。K8s 客户端提交 AgentWorkload CR，控制器只创建/更新关联 StatefulSet/Headless/PVC；副本状态来自 Kubernetes。

## Control 元数据接口

| 方法 | 路由 | 一致性边界 |
|:---|:---|:---|
| ConfirmMetadataSelection | POST /api/v1/workspaces/{workspace_id}/agent-metadata:confirm | expected_version + idempotency_key + 完整选定资源引用；本地事务写不可变 release/选择序号/通知 Outbox |
| GetConfirmedMetadata | GET /internal/workspaces/{workspace_id}/agent-metadata | 配置 ID、目标选择序号/版本；返回有效或 superseded/current；历史固定版本另用 for_session 查询 |
| ReportMetadataApplication | POST /internal/workspaces/{workspace_id}/agent-metadata/results | 已验证 Gateway 实例上下文；report_id 冲突校验与实例代次投影 |
| GetMetadataProgress | GET /api/v1/workspaces/{workspace_id}/agent-metadata/progress | 当前选择 + 当前已登记实例；不把 ACK 或空实例集合当成功 |

选定资源引用包含 namespace/kind/id/version/checksum/content_ref，禁止 latest。Prompt 文本与非敏感配置进入不可变 release，Skill 内容按 checksum 取得不可变对象；秘密只能按凭据引用在执行时重取。MCP Binding/OAuth 的权威所有者仍为 Plugin/Auth。

## Agent Session/执行所有权接口

现有注册、SubmitTurn、GetTurn、Interrupt、Messages、Events、Agent Request 接口保留。注册增加 workspace/agent_configuration/实例/固定版本引用。所有执行写请求绑定当前执行 owner 与 epoch；事实查询不要求旧进程存活，可由该 Workspace 的其他就绪实例提供。

| 方法 | 路由 | 契约 |
|:---|:---|:---|
| BeginDrain | POST /api/v1/runtime-instance:drain | operation_id/expected incarnation；持久化关闭新执行准入，running 继续、queued 不再 claim |
| GetDrain | GET /api/v1/runtime-instance/drain | drain revision、running count、owned Session IDs；只有准入屏障生效且本代次任务/flush 完成才 quiesced |
| QuiesceSession | POST /api/v1/runtime-sessions/{session_id}:quiesce | transfer_id；只阻止该 Session 领取后续 queued，当前 running 和其他 Session 继续 |
| TransferSession | POST /api/v1/runtime-sessions/{session_id}:transfer | expected owner/epoch + target verified instance；Agent TX 锁 Session，无 running 时转移，否则需环境终止证据后 failed+paused，不重放 |
| AdoptSession | POST /api/v1/runtime-sessions/{session_id}:adopt | 已提交 owner/epoch；加载原 handle，按原 FIFO 继续；原 paused 状态不变 |
| ResumeSession | POST /api/v1/runtime-sessions/{session_id}:resume | 签名携带 expected epoch，正文 operation_id；用户由 Gateway /api/v1/sessions/{session_id}/resume 显式确认；恢复原 queued 队列，不重放 failed Turn |

Gateway 先持久化 binding phase=transferring 与稳定 transfer_id，再调用 Agent Transfer，查询 Agent 实际 epoch 后收敛 Gateway active binding；失败/超时恢复原 transfer，不随机再迁一次。不能在跨库事务内同时更新两侧。k8s NotReady 不作为 running 已结束的证据；正常 drain 不需停止整个 Pod 后迁移空闲 Session。

## MCP 与 Session 凭据

| 方法 | 路由 | 契约 |
|:---|:---|:---|
| Register/UpdateDefinition | POST/PUT /api/v1/mcp-definitions[/id] | 管理员；稳定 slug、受控服务/endpoint/auth、expected_version |
| List/GetDefinition | GET /api/v1/mcp-definitions[/id] | 受控目录查询，不暴露后端秘密 |
| HandleMCP | POST/GET/DELETE /mcp | 标准 MCP 协议；Session Token 授权，Gateway 聚合发现并代理 |

对外工具名由稳定 Definition slug + 原工具名构成，禁止同名覆盖。分页 cursor 绑定 Session、目录/Binding 版本与位置。调用时重新校验当前授权与禁用状态。prompts/resources 保留来源映射；入口无状态、上游请求作用域连接，不保存必须跨请求恢复的协议引用；未来后台订阅须另行设计引用持久化，不以 Gateway 进程内连接建立授权。发现/只读允许重试，未知写结果不自动重试。外部 OAuth 仍经 Plugin 适配，Agent 只配置 Gateway 地址。

Session Token 与 Web Cookie/Agent API Token 分离。Gateway 持久化 Token Hash 与生成密钥版本引用；通过持久 Secret key + Session ID 的确定性 MAC 生成稳定 opaque token，不持久化明文、不作为 JWT。密钥轮换保留旧 Session 所用 key 版本，撤销由 Registration 决定。

## 元数据产物下载

Gateway 增加 GET /api/agent/v1/metadata-artifacts/{checksum}，核对已验证实例与允许 release 的资源引用后代理存储字节；不让 Agent 配置任意对象地址。下载可安全重试，返回 hash/长度；Agent staging 校验后发布，禁止解包路径逃逸和符号链接越界。

## RabbitMQ 契约

- Session exchange：durable topic media.agent.sessions.v1；routing key 使用 session_id 的十六进制 UTF-8 编码，禁止通配字符歧义。消息保持 session_id/sequence/payload，sequence=0 表示 delta。
- 每 Gateway 实例一个 server-named exclusive auto-delete 队列。一个实例消费循环调用本机 Broadcaster；本机第一订阅建立绑定，最后订阅释放绑定，SSE 只创建本机接收者。
- Source.Open 先注册本机接收者、确认 queue binding 后才返回，随后 replay。重连恢复活跃绑定，本机订阅不成为 Broker 队列。
- 慢消费者只终止该本机接收者；Broker 通知不是数据库事实。实时发布不加 Outbox，保持已确认的 replay/周期补查恢复。
- Metadata exchange：durable topic media.agent.metadata.v1；按 workspace/config 编码路由，每个实例独立临时队列。Control 的确认通知有 Outbox，Agent 串行应用、延迟重试、启动/重连及60秒对账补齐已确认目标。
- 消费未知主版本/损坏消息隔离记录，不热循环；通知失败不改变已提交事实。配置结果走 API，不新增结果队列。

## 具体创建规格

AgentWorkload CR：apiVersion runtime.media-agent.io/v1alpha1；spec 包含 workspace_id、agent_config_id、image、replicas、volume_ref、resources、Gateway/Broker 连接及 Secret 引用。稳定资源名以 Workspace 安全 hash 生成；ownerReferences 指向 CR，PVC 使用 Retain 策略，不因 CR 删除清理用户数据。Gateway 缩容修改 CR.spec.replicas，禁止直接 patch StatefulSet 与 Controller 相互覆盖。

Docker service：PUT /v1/workspaces/{workspace_id}/agent-container，使用稳定创建键和允许镜像/挂载根；返回 container_id/base_url。重复请求配置不兼容返回409；不创建多副本，不将 Docker socket 提供给 Gateway。


## 目录与依赖

Go HTTP 接口源分别放 idl/media_agent_gateway/agent_runtime.proto、mcp_catalog.proto，idl/media_control/agent_metadata.proto、idl/media_runtime_controller/container.proto，双注解定义路由；hz/protoc 生成 DTO/router/client/swagger，不手改生成文件。MCP raw JSON-RPC handler 使用 IDL 路由，在 handler 转协议请求后调用 workflow。

Gateway 增加 domain/agent_instance、mcp_catalog；service 仅操作各自聚合；workflow 负责创建、迁移、MCP 与配置查询代理。AMQP/Kubernetes/Docker 的具体实现放共享基础设施及 client 装配，禁止 service/domain 依赖 SDK 或 generated DTO。

Agent 增加 runtime_metadata Domain/use case、notification workflow、版本产物 adapter；现有 runtime_session 增加实例 owner 与 epoch，并迁移 SQLAlchemy/Alembic。新 Python API 同时维护 YAML OpenAPI 和 Pydantic schema。

media_runtime_controller 包含独立 K8s 自定义控制器与 Docker 创建服务两个入口；K8s 控制器仅协调 AgentWorkload 的关联资源，Docker 接入遵循 Hertz IDL/handler/client 规范。本模块不查询 Agent 业务或拥有 Turn 表。

## 数据迁移

### Gateway

agent_sessions 增加 agent_config_id、environment_id、replica_id、incarnation_id、binding_version、binding_phase(active/transferring)、transfer_id、target_replica_id、target_incarnation_id、expected_execution_epoch、token_hash、token_key_id。现有 agent_id 类型标签保留兼容，不把它当委托配置 ID。新字段采用先可空、回填/首次授权协调、再约束的迁移方式，不伪造旧 Token。

agent_instances 字段为 environment/workspace/replica 主键、config/incarnation/resource_uid/endpoint/admission/drain_operation/version/credential_hash。mcp_definitions 为稳定 slug 主键、服务身份、受控 endpoint、transport/auth、status、version。当前无跨请求上游会话，不建立 mcp_upstream_sessions；历史定义、目录 revision 和变更 Outbox 由 MCP Catalog 保存，不保存凭据或 Control 配置。

### Control

agent_metadata_releases 保存 config/workspace、context_version、selection_revision、版本向量、非秘密完整 manifest、创建者、idempotency_key/请求摘要；config+revision 和 config+key 唯一。同 key 不同选择返回409。

catalog_resource_versions 保存各 Control-owned Catalog 当前版本的历史内容/不可变引用，当前资源更新与历史写入同本地事务。已有数据只回填真实当前版本，不捏造更早历史。Skill 内容按 sha256 保存不可变对象，manifest 只含内容引用，不含模型密钥。Provider 凭据历史采用加密版本引用，执行链路按权限取密文解密，不写入 release 明文。

metadata_notification_outbox 保存稳定事件 ID、scope、封套/数据体、交付状态/次数/下次时间；metadata_application_reports 保存稳定 report_id、scope、selection/target、replica/incarnation/status/error/hash。实例汇总读取当前 Gateway 实例集合，0个实例显示等待运行，不能算成功。

汇总目标成员为工作负载当前期望的非 draining 副本，按 replica/incarnation 关联已验证注册；未创建、未就绪或尚未注册的期望副本仍为 pending，不能从分母删除后宣称全部成功。扩容增加成员，新代次替换旧代次；已完成缩容的副本从当前视图移除，历史结果保留。确认后300秒尚无明确结果时返回 timed_out=true 的等待状态；超时不伪造 failed，不停止对账，后续真实结果仍可收敛。

### Agent

runtime_sessions 增加 workspace_id、agent_config_id、owner_instance_id、owner_incarnation_id、execution_epoch。runtime_instance_admission 保存当前副本/代次的 active/draining 与排空 operation。执行 claim、finalize、持久事件都在 Session 行锁事务核对 owner/epoch，partial unique running 约束保留。

实例应用/待上报 marker 位于 Workspace 内按实例代次隔离目录，原子写入；发布产物按 context/hash 唯一。配置目录不作为用户工作 cwd，HOME 仍共享；Codex 通过固定 CODEX_HOME/会话配置及 Claude 的固定参数引用加载，不覆盖 /workspace 全局旧 Prompt。原生 session handle 保留。

## 消息配置与恢复

Go 使用官方 amqp091-go，Python 使用 aio-pika robust。RabbitMQ topic exchange 与名称见本规格接口节。Gateway 一个实例队列，由 Source Open 的活跃 Session 引用计数绑定；Broker reconnect 恢复绑定，连接关闭不会关闭其他本机接收者。max-length=10000、消息 TTL=30000ms、receiver buffer=256；满载淘汰临时通知并依赖持久 replay；delta 可丢。

Metadata 通知实例队列使用独立 exchange，max-length=1000、TTL=300000ms；队列断线/实例重启允许重建。Control Outbox 有限指数退避最大30秒，Agent 对账周期60秒。确定格式错误隔离并 ACK，瞬时错误以延迟调度重试，不同步无限 requeue。确认通知与传输 ACK 没有分布式事务。

配置项在 idl/config/config.proto 的 rabbitmq、agent_metadata 与后续 agent_runtime 段定义，YAML/env 仅作为加载源；Python 对应 event_bus.rabbitmq_url/session_exchange 与 instance/gateway 配置。Agent 事件链路的 Redis 依赖与部署变量完全移除，其他服务的共享 Redis 配置不属于本次范围；in-memory 只供独立单测/明确 mock，不作为分布式生产替代。

## 创建资源与扩缩容

K8s Gateway 使用 dynamic/core 客户端提交/查询 AgentWorkload CR。Controller reconcile 只读取 CR 和它管理的资源，构建单 Workspace StatefulSet、Headless Service、RWX PVC、固定 Secret 引用及探针。StatefulSet 使用 Parallel、共享同一 PVC、挂载 /workspace；PVC Retain，不挂 controller ownerReference 自动删除用户文件。资源冲突必须校验 Workspace 标签和 owner UID，不覆盖外部对象。

Gateway 根据 observed Ready Pod 与已登记 instance 选择新 Session。缩容先持久业务 admission/drain operation，代理 Agent 关闭 claim，排空后接管 queued，再更新 CR replicas；Controller 把目标落到 StatefulSet。不直接在 Gateway 修改 StatefulSet 与 CR 互相竞争，不在 Controller 执行排空业务。

Docker 创建服务按 Workspace 派生唯一容器名和幂等标签，固定允许镜像与持久挂载根，禁止用户指定任意 privileged/socket/host路径。创建结果不明确以名称/标签读取，单实例；不做 Docker 扩缩容。

## 完整流程实现顺序

1. Source 替换与部署配置；保留 Web 与 Agent API 行为，验证真实跨语言消息。
2. Control 不可变 releases/history/Outbox，Gateway 受控查询与上报，Agent 版本产物与通知应用。
3. Session Credential、实例登记、owner epoch、Scoped recovery、迁移/drain/resume与路由。
4. MCP Catalog/身份过滤/协议代理和独立 Platform Prompt；Plugin/Auth 外部所有者保留。
5. Kubernetes CRD/Controller/Gateway 创建接口以及 Docker 单容器创建服务。
6. 真实端到端、迁移回填与 Harness gates。

## 验证门禁

伪代码模型22场景仅证明抽象流程一致；必须完成真实 PostgreSQL/RabbitMQ 两 Gateway/两 Agent 广播与 replay，重复通知/旧代次回报，接管竞争/响应丢失/FIFO，MCP授权变更/命名冲突/超时不重试写，Control明确选择后的不可变文件隔离，以及 Kubernetes资源reconcile和Docker单容器创建。

Go 各受影响 module 运行 gofmt、go test ./...、make arch-check。Python 运行 ruff check/format、mypy、lint-imports、vulture、pytest。IDL生成后 swagger/client/schema 与源码一致。计划记录实施状态，不能以文档或 fake API 的成功宣称真实部署已完成。

P2 元数据接口、受控实例凭证、版本产物及部署参数见 [元数据通知规格](agent-metadata-notification.md)，验证与阶段边界见 [P2 审查](../review/agent-runtime/p2-metadata.md)。

P3 实例凭证、签名、Session Token、admission 与接管细则见 [Session 所有权规格](agent-session-ownership.md)。
