# P5 · Agent 承载资源与创建

Control 持有完整 Workload Target，Gateway 每次读取授权后的版本化目标；用户只能更新 replicas，不提交 manifest、任意 Secret、image、host 挂载。Controller 仅协调原生资源，没有 Agent SDK/业务 HTTP/数据库依赖。

Control 新增受控 Workload Target 查询与副本更新，目标及请求幂等记录同事务，后台 Gateway 对账确保已提交目标最终进入环境。Gateway Ensure/Get/Scale 查询 Control 当前目标和版本；K8s 只提交/读取 AgentWorkload CR，缩容以 CR CAS 保存目标版本/operation 后协调实例 admission、Agent drain 和 Session 转移，完成后再降低 spec.replicas。

K8s CRD 为 runtime.media-agent.io/v1alpha1 AgentWorkload；Workspace/环境 hash 构成稳定名。Spec 固定 workspace/config/environment、Agent 类型、镜像、端口、namespace、identity/shared Secret 引用、Gateway URL、RWX claim、StorageClass/容量、CPU/内存；变更不携带秘密。Controller 创建/协调 StatefulSet Parallel、Headless Service（稳定逐副本DNS）、RWX PVC；PVC 无 GC owner 引用，PV reclaim Retain，探针交给 kubelet。Pod 使用 projected Pod-bound 启动 token、公钥文件、随机运行 incarnation，Workspace/HOME 持久共享，运行数据库和 Broker/稳定 Agent Token 通过 Secret 提供。ServiceAccount 不具备 Kubernetes API 权限，Gateway 与 Controller 使用分离 RBAC。

Docker 创建入口是独立 Hertz 服务。Engine adapter 通过 Unix socket 调 API，不给 Gateway socket；只准批准镜像、稳定 Workspace 目录/受控 Secret 根、固定网络及HTTP端口，不接受 privileged 或任意命令。创建稳定名及配置摘要，检查归属与挂载；重试先inspect，冲突不覆盖，单实例。启动证明存于私有根、校验 hash，原生generation来自 container id + startedAt；容器健康由 Docker healthcheck/restart 维护，不把进程状态查询做成 Agent 业务探测。

验证使用 client-go fake API 及 Controller reconcile，真实可用 Kubernetes API 的资源创建/状态检查，Docker fake Engine 与独立真实单容器创建；缩容需要两个 Gateway 的并发/重试和 running/queued/未知旧执行场景，保持 P3 不重放不明执行规则。

## 存储与授权

Control migrations/000023 创建 agent_workload_targets 与 agent_workload_requests。目标包含 workspace/config/environment/owner、不可变 spec snapshot、用户 desired_replicas、生效 replicas、Workspace version 和 target version。相同幂等键/请求 hash 返回原接受结果，不代表它仍是最新目标；不同 hash 拒绝。副本更新 CAS 与请求记录在同一本库事务提交。Control Workspace manage 是现有实际权限动作；查询/ensure/更新均最终由 Control 校验，内部列表只允许已验证的 system 服务身份。停止投影为0，保留用户 desired_replicas，恢复运行时重投影；目标创建后不隐式滚动修改镜像或 Agent 类型。

Gateway 的 WorkloadProvisioning 只协调客户端/Domain 服务。共享 spec wire contract 在 hertz_infra/agentworkload；native SDK 实现在 biz/shared/client。Native resourceVersion 防止多个 Gateway 覆盖执行进度；Control version 决定新执行意图。已开始的排空操作完成后再读取更晚目标；native spec.replicas 是已安全提交的数量，pending-replicas/pending-version/drain-operation annotations 保留未完成步骤。Gateway agent_instances admission 追加 retired；该序号新原生代次登记可 active，但排空过程中的重启不能清除 draining。

缩容到0也逐个查询绑定 Session 的执行事实；不能把当前 incarnation 的 running_count=0 当作旧 incarnation 已完成。先恢复旧代次绑定，再施加实例屏障；不明确时维持承载数量。降低 native 数量后等待 StatefulSet 采纳且被移除 Pod 从资源集合退出，才清除排空标记。元数据 membership 按 Control 生效副本数列出所有预期序号，未登记/不运行者保留空 incarnation 占位，不误判全部应用。

## 接口

| 所有者 | 入口 | 行为 |
|---|---|---|
| Control | POST /internal/workspaces/{id}/agent-workload:ensure | 授权并建立/投影当前目标 |
| Control | GET /internal/workspaces/{id}/agent-workload | 读取已存目标 |
| Control | PUT /api/v1/workspaces/{id}/agent-workload | replicas、expected_version、idempotency_key；Docker拒绝此副本更新 |
| Control | GET /internal/agent-workload-targets | system 服务分页读取 durable targets |
| Gateway | POST /internal/agent-workloads/{id}/ensure | 按当前 Control 目标协调，未完成202 |
| Gateway | GET /internal/agent-workloads/{id} | 当前目标、已提交数量、native Ready、排空操作 |
| Gateway | POST /internal/agent-workloads/{id}/scale | 只接受与 Control 当前 expected_version/replicas 匹配的执行请求 |
| Docker Controller | POST /v1/workspaces/{id}/container:ensure | 受 creation credential 保护，observation 或 revision CAS 协调单实例 |
| Docker Controller | POST /v1/workspaces/{id}/container-identity | 受 creation credential 保护，校验自身 proof 或读取 native 状态/代次 |

IDL 均使用 hz 生成 DTO/client/router，Swagger 使用 protoc。内部服务凭据只用于受控 RPC；Agent API Token、Session Token、instance credential 和 native bootstrap proof 保持分离。

## 配置与交付

Control/Gateway 的 agent_workload.enabled 默认关闭，Gateway 必须同时启用 agent_runtime。Control 的批准 profiles 包含 codex、claude-code、goose 的 image/port/cpu/memory，另配置 driver、namespace、gateway_url、shared_secret、storage_class、storage_size。Controller Kubernetes policy 单独批准 namespace/shared Secret/Agent image，不读取 Control。

Kubernetes shared Secret 交付数据库/Broker URL 与 runtime-public-keys.json；稳定命名的 Workspace identity Secret 交付 Auth 已签发的 agent-api-token。Docker 同样由私有部署 policy AgentTokens[configuration_id] 提供，不把模型 Provider API Key 用作稳定 Agent Token。Token 获取和撤销仍由 Auth/已有管理流程拥有，创建不会擅自签发委托凭据。

Docker 文件/容器 UID/GID 与创建服务一致，Root 私有0700、proof只读0400，持久 Workspace可写。创建请求没有 privileged、command、host mount 等用户输入。API 协商1.41–1.45；native stop 的120秒 grace 使用匹配的 HTTP 等待期限，已提交 intent 通过资源协调重试，不回放未知业务执行。证明旧代次终止同时要求 exact generation、非 running/非 restarting、FinishedAt >= StartedAt。

部署和验证入口见 media_runtime_controller/README.md。真实资源验证与完整 Agent 模型调用分开：P5 的真实容器/Kubernetes资源与 P3/P4 的真实 Go/Python Agent/Broker/数据库链路共同覆盖创建与缩容；P6再做完整部署验收。缺失 RWX provisioner 时明确记录 Pending，不能据此声称已验证双 Ready/共享文件。

Broker exchange 是 Workload spec 的可选受控字段：sessionExchange 来自 Control agent_workload.session_exchange，metadataExchange 来自 Control agent_metadata.metadata_exchange。Kubernetes/Docker 都显式传到 Agent 对应环境变量；缺失的既有目标沿用共享默认值。Control 启用 workload 时必须同时启用 Agent Metadata。
