## agent 架构设计

当下 media_agents 里实现了各种 agents，media_aget_gateway 作为 agent 的网关。

### 会话事件总线替换
当前实现使用 PostgreSQL 保存执行事实，使用 Redis Pub/Sub 分发会话事件。目标是以 RabbitMQ 完全替代 Redis；各 Gateway 维护本地 SSE 连接，执行与恢复事实不依赖 Gateway 实例。

经过调研和分析，决定引入 rabbitmq，核心目的有两个:
1. 完整消息、工具结果快照和 Turn 生命周期事件先提交 PostgreSQL，再投递 RabbitMQ。文本 delta 只实时传输，不落库、不占恢复序号、不参与回放。
2. 每个 Gateway 实例对应一个 RabbitMQ 会话事件临时队列，消费后交给服务内 Broadcaster 按 Session 广播给本机的多个 SSE 连接。每个 SSE 只拥有本地接收者，不单独创建 RabbitMQ 队列；关闭单个 SSE 不关闭实例队列。
3. Gateway 先订阅并缓存 RabbitMQ 事件，再通过 Agent 查询接口回放 PostgreSQL 中的历史事件，按 Session Sequence 去重和补齐；不直接访问 Agent 数据库。允许提交后投递失败，沿用回放与周期补查恢复
4. 每一个 Gateway 都可以接受用户发送的 chat，都可以直接发送给 agent，chat 消息在 agent 内排队
5. agent 状态也由 agent 自己维护，并负责更新到数据库


### 元数据变更通知

RabbitMQ 承载通用元数据更新通知，覆盖 Prompt、Skill、Memory、MCP 配置等。只有用户在 Control 明确选择并确认更新后才发通知；资源目录编辑本身不自动升级运行配置。

链路：Control 保存已确认版本并发布通知 → Agent 接收 → Agent 经 Gateway 拉取 Control 的已确认配置 → Gateway 在返回链路仅生成 Platform Prompt 正文 → Agent 版本化落盘并准备加载。Gateway 不缓存控制配置。已有 Session 和 queued/running Turn 不自动改变固定版本；共享文件不能原地覆盖旧 Session 依赖。

协议使用统一封套与按 type 独立版本的数据体。当前 agent.metadata.update_requested 的 data 携带 selection_revision、target_context_version 和可选变化提示；scope 指定 Workspace 与稳定 Agent Configuration ID。用户显式回滚资源仍递增选择序号；未知变化提示不阻止完整配置拉取，完整配置按统一响应 schema 校验，平台代码负责版本化产物及 Runtime 适配。消息不携带正文或凭据。详见 [协议设计稿](../../spec/agent-metadata-notification.md)。

Skill 版本化由系统控制：Control 保存选定版本与配置历史，Agent 侧的平台代码准备版本化文件并原子发布，Runtime adapter 完成具体加载；不将版本管理或旧 Session 隔离作为 Agent 可选能力。

更新结果由 Agent 经 Gateway API 上报 Control，包含选择序号、配置版本、副本 ID、运行代次与成功/失败信息。Control 保存实例应用投影并展示更新中、全部成功或部分失败；旧实例晚到结果不能覆盖新实例，Broker ACK 不代替业务结果。

### Agent 身份与执行实例

Agent Identity 是配置对应的稳定委托身份；Agent Runtime Instance 是可替换执行实例。二者与 Session、Runtime Handle 分别建模；Session 记录配置、replica/incarnation 与 execution_epoch，实例登记和接管以原生资源代次及数据库 CAS 校验。

### agent_gateway 定位

Agent 只连接 Gateway 的统一 MCP 入口。Gateway 对 Agent 提供 MCP Server 接口，发现并呈现 Agent 有权访问的工具，再将调用代理到具体 Plugin/MCP 实现；具体工具实现和最终资源授权归后端服务。

Gateway 维护平台模板，根据 Agent 类型及已授权的 Skill、MCP、Memory 等输入，仅生成 Platform Prompt 正文。Control 持有用户 Configured Prompt 的内容与版本；Gateway 不把两类 Prompt 合并为唯一最终提示词。Agent 分别接收两类内容，负责 Runtime 特有的配置转换、文件落盘和加载。此职责迁移不意味着运行中热更新，Session 固定版本规则保持不变。

MCP 协议支持 tools/list 工具发现与 tools/call 调用；服务目录发现可参考官方 MCP Registry 的 REST API 与 server.json。注册目录归 Gateway，Plugin 负责具体 MCP 实现；工具、Prompt、资源和模板按当前权限过滤；Binding/Definition 配置固定版本，每次请求重验当前权限和停用状态，不隐式改变 Session 固定配置版本规则。入口兼容 server/discover 与 initialize，按请求独立协商上游协议。

参考：
- https://modelcontextprotocol.io/specification/2026-07-28/server/tools
- https://github.com/modelcontextprotocol/registry/blob/main/docs/modelcontextprotocol-io/about.mdx
- https://github.com/docker/mcp-gateway
- https://github.com/microsoft/mcp-gateway

### agent pod 的创建

Workspace 可有多个 Agent Pod，对 Workspace 下的 Session 分配负载。参考 agent-platform：
1. 一个 Workspace 对应一个 StatefulSet 与 Headless Service，多个 Pod 共享 Workspace RWX PVC（包括 HOME），不使用每 Pod 独立 volumeClaimTemplates。
2. Session 与副本 ordinal 的绑定存入数据库，Gateway 按逐 Pod DNS 路由。绑定 Pod Ready 且未 draining 时沿用；扩容不改变已有有效绑定。
3. 支持缩容：目标副本先 draining，完成当前 running Turn 后停止领取下一条；queued Turn 保持原 FIFO，由后台协调 Session 重绑后继续执行，原暂停状态保持不变。绑定 Pod 不可用或 draining 时可协调重新分配 Session。
4. 互斥在 Session 级别：同一 Session 最多一个 running Turn，不同 Session 可跨 Pod 并行。共享配置/Skill 文件另需原子发布；所有副本不能扫描并恢复其他副本的执行。
5. 本项目保留 Agent-owned 持久化 Turn 队列，避免照搬旧项目 Control-owned claim；重绑时不自动重放结果不明确的工具执行。

### Runtime Controller 与创建入口

Control 持有 Workspace、Agent 配置与 Placement；Gateway 使用这些权威配置创建 Agent 资源并直接与 Agent 通信。

Kubernetes：
1. Gateway 使用 Kubernetes 客户端提交 Agent 自定义资源 manifest。
2. 自定义 Runtime Controller 创建/协调 StatefulSet、Headless Service 与 Workspace RWX PVC 等承载资源。
3. StatefulSet 维护副本，kubelet 执行探针、重启与容器生命周期；Runtime Controller 不探测 Agent 业务/健康状态，不实现独立副本调度或接管判断。
4. Gateway 使用 Kubernetes 客户端读取资源和 Pod 状态，按 Session 绑定直接调用 Agent。
5. 业务排空与 queued Turn 重绑由 Gateway/Agent 协调，完成后提交副本目标，由 StatefulSet 落地。

Docker：
1. Gateway 调用有 Docker 权限的 Runtime Controller 创建服务，为 Workspace 创建单个容器。
2. 服务返回容器引用和连接地址；Gateway 直接与 Agent 通信。
3. 本期不考虑 Docker 的多副本与扩缩容，不为其引入 Kubernetes 式调度/探活协议。

普通重启保留 Workspace 数据，删除/rebuild 的存储回收遵循明确生命周期策略。创建工作流详见 [agent_workload_provisioning](../../ddd/media_agent_gateway/workflow/0009-agent_workload_provisioning/agent_workload_provisioning.md)。
