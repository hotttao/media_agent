# Agent 元数据更新通知协议

状态：V1 协议规格。P2 的 Control/Gateway/Agent 链路已实现并在隔离 PostgreSQL/RabbitMQ 环境验证；阶段状态及后续依赖见 [P2 审查](../review/agent-runtime/p2-metadata.md)。

## 边界

使用统一消息封套和按 type 独立定义的数据契约。当前实现目标是用户确认元数据更新后的 Agent 配置拉取，不把生命周期命令、工具调用或会话事件混入该消息类型。

Control 保存用户明确确认的选择，Agent 经 Gateway 查询 Control 的有效配置；Gateway 不缓存控制配置，仅在响应链路生成独立 Platform Prompt 正文。目录发布新版本不自动更新运行选择，已有 Session/Turn 保留原配置。

依据：[agent_metadata_update](../ddd/workflow/0010-agent-metadata-update/agent_metadata_update.md)、[platform_prompt_assembly](../ddd/media_agent_gateway/workflow/0008-platform_prompt_assembly/platform_prompt_assembly.md)、[Control Harness](../../harness/contributing/architecture.md)、[Agent Harness](../../media_agents/adapter/contributing/architecture.md)。

## V1 消息封套

Content-Type 为 application/json，UTF-8。AMQP message_id 等于正文 event_id；去重键为 source + event_id。重投保留两者，同一生产方不得将同一 event_id 用于不同消息内容。

```json
{
  "schema_version": 1,
  "type": "agent.metadata.update_requested",
  "data_version": 1,
  "event_id": "evt-001",
  "source": "media_control",
  "occurred_at": "2026-10-11T08:00:00Z",
  "scope": {
    "workspace_id": "ws-001",
    "agent_config_id": "cfg-001"
  },
  "data": {
    "selection_revision": 18,
    "target_context_version": "42",
    "changes": [
      {
        "kind": "media_control.skill",
        "resource_id": "skill-001",
        "resource_version": "7",
        "action": "upsert"
      }
    ]
  },
  "trace_id": "example-trace-id"
}
```

| 封套字段 | 必填 | 契约 |
|:---|:---|:---|
| schema_version | 是 | 封套主版本，V1 为 1；只因封套不兼容变化而升级 |
| type | 是 | 消息类型；与 data_version 一起选择数据解析器 |
| data_version | 是 | 本 type 的数据主版本，独立于其他消息类型 |
| event_id | 是 | source 内唯一且稳定的通知 ID，不是排序游标 |
| source | 是 | 已登记服务标识；当前更新类型只允许 media_control 发布 |
| occurred_at | 是 | UTC RFC3339，仅用于观测，不决定版本顺序 |
| scope | 是 | 路由和作用域描述；字段要求由具体 type 决定 |
| data | 是 | 具体 type 的业务数据，不在封套硬编码目标配置字段 |
| trace_id | 否 | 诊断关联，不参与身份、权限或幂等 |

封套可以支持不同服务和未来消息类型，但不意味着任何服务都可发布任意 type。生产权限绑定 source/type/路由；正文 source 不能自行授予权限，消费者仍需核对队列范围，Gateway/资源服务执行拉取授权。

同一业务变化影响多个 Workspace 时为各 scope 创建独立通知 ID；对同一 scope 的投递重试复用原 ID。不得将不同 scope 的消息当成相同 source/event_id 重投。

## agent.metadata.update_requested / data_version=1

| 字段 | 必填 | 契约 |
|:---|:---|:---|
| scope.workspace_id | 是 | 一个消息对应一个 Workspace |
| scope.agent_config_id | 是 | Agent Configuration 稳定 ID，不是 Runtime 类型标签 |
| data.selection_revision | 是 | 同 workspace/agent_config 内的正整数选择序号；来自 Control 配置 version |
| data.target_context_version | 是 | Control 已持久化的配置引用，非空不透明字符串；Agent 不解析或比较其大小 |
| data.changes | 否 | 变化解释信息，可省略或为空；不是 patch、授权列表或消费调度依据 |

changes 元素包括 kind、resource_id、resource_version、action。kind 使用来源命名空间，例如 media_control.skill、media_control.configured_prompt、media_control.memory_binding、media_plugin.mcp_binding、media_agent_gateway.platform_prompt_template。新增提示类型不要求升级封套或 data_version。resource_version 是权威来源的版本标识字符串，不跨资源比较；action 为 upsert/remove，未知提示 action 可忽略。

消费者遇到未知 changes.kind 只忽略该项提示并继续拉取完整目标配置，不按消息自行执行安装或删除。完整配置按系统定义的响应 schema 校验，未知必填结构或无法加载的产物明确失败，不能忽略后宣称已应用。

selection_revision 表达用户选择顺序，不代表资源版本。用户先选 Skill v7、随后明确回滚 v5：选择序号仍从 18 增加到 19，这是一项有效更新。旧消息携带选择序号 18 才是过期通知。

若未来事件只涉及 Workspace、不对应 Agent Configuration，应定义相应 type 及 scope 契约，不把当前消息的 agent_config_id 变成可猜测的空值。实例/Session 范围消息也需单独定义语义，不隐式改变本消息的 Session 生效规则。

## 发布与路由

使用独立元数据通知 Exchange，与会话事件队列隔离。当前消息按 workspace_id + agent_config_id 路由，每个在线 Agent Runtime Instance 使用独立队列；同 Workspace 多 Pod 各收到一份，不竞争消费，不按 SSE/Session 创建通知队列。

Control 用户确认提交后才发布，构建完整目标配置失败时不发布无效目标。消息不携带配置/Prompt/Skill 正文、Token、Provider Key、任意 Endpoint 或脚本。Broker 与应用两层都拒绝实例通过伪造 scope 更新其他 Workspace。

实例启动、Broker 重连及周期对账经 Gateway 查询用户当前已确认的选择；未在线时不依赖完整通知历史。对账只补齐确认过的选择，不读取 Catalog latest 自动升级。Exchange 为 durable topic；每实例 server-named exclusive/auto-delete 队列，max-length=1000、TTL=300000ms、prefetch=1。Control Outbox 以15秒 lease 领取，publish 上限5秒，失败延迟2至30秒重投；Agent 启动、重连及每60秒对账。未知/损坏协议记录脱敏错误并 ACK，不无限 requeue。

## 拉取与统一配置契约

Agent 向 Gateway 提交经验证的 scope、selection_revision 和 target_context_version。Gateway 代理 Control 指定选择查询，不缓存。Control 返回：目标仍有效、被后续选择替代、无权限、目标不存在或暂不可用。

superseded 响应必须说明当前已确认 selection_revision 与配置引用；不存在不等于被替代，不能自行跳到 Catalog 最新版。版本标识由权威服务解释，不按 event_id、时间戳、资源版本或 target_context_version 排序。

成功响应包含 selection_revision、固定资源版本/内容校验值、独立 Configured Prompt 与 Platform Prompt 及模板版本。metadata_schema_version 只定义配置响应的结构版本；未知可选字段可忽略，不支持的结构主版本报告 incompatible，加载/校验失败报告 failed。不得引入 required_capabilities 或 metadata_capabilities 让实例选择是否支持平台版本管理。

版本选择与历史配置由 Control 管理，内容校验、版本化文件准备和原子发布由 Agent 侧的平台加载代码实现，具体 Runtime adapter 负责格式转换和加载位置。Skill 版本化与旧 Session 隔离是所有相关实现必须满足的系统契约，不是模型或 Agent 自主声明的能力。不同 Runtime 的实现差异通过 adapter 和部署验证解决，不能以可选能力声明绕过不变量。

Provider Key 等秘密通过现有受控执行信封传递，不进入正文、消息或持久日志。HTTP 路由与 DTO 由 `idl/media_control/agent_metadata.proto` 与 `idl/media_agent_gateway/agent_metadata_gateway.proto` 生成，使用专用受控入口。

## 消费与应用规则

1. 分别校验封套版本、type、data_version 及 scope；未知封套/消息类型/数据主版本进入隔离观测，不猜测执行。
2. 以 source/event_id 去重；已应用状态按 workspace/agent_config 保存 selection_revision 与配置引用，不能仅依赖进程内已收消息集合。
3. 同一实例、同一配置作用域串行协调，合并重复目标；不同作用域可独立执行。同一配置下不同资源保持完整快照应用，不能按 changes 随意拆成并行 patch。
4. 按统一 schema 拉取并校验全部产物，准备完整版本化文件；已有 Session 继续引用旧版本。
5. 准备完成后再经 Gateway 核对已确认选择：若已 superseded，弃用候选并拉取新的已确认目标；若仍有效，在本机串行提交已应用版本与新 Session 可用指针，不降低已应用 selection_revision。
6. 单实例事务不能与 Control 组成分布式事务。Control 恰在最后核对后出现新选择时允许短暂应用旧有效目标，再由新通知/对账收敛；不得宣称任意时刻都等于 Control 最新选择。
7. 接纳处理提示后 ACK；瞬时错误唤醒延迟对账，并由启动/周期查询修复丢失提示。成功与失败的业务结果另行持久化并上报，不依赖 Broker 无限 requeue。Broker ACK 不代表整个 Workspace 更新成功。

共享卷采用版本化产物和原子发布。跨实例不能写同一个全局可变指针后互相覆盖；共享产物按内容版本幂等发布，已应用/可用状态按实例及配置作用域记录。Codex 通过业务 Session 专属 CODEX_HOME 加载固定 Skill、平台 AGENTS.md 与独立 developer_instructions；Claude 使用固定版本的 append-system-prompt-file 与 plugin-dir。

## 版本演进规则

| 变化 | 是否升级 |
|:---|:---|
| 新增可选封套字段 | 不升级 schema_version，旧消费者忽略 |
| 新增独立消息 type | 不改变已有 type/data_version；旧消费者隔离未知类型 |
| 现有 type 新增可选数据字段 | 不升级 data_version |
| changes 新增提示 kind/action | 不升级；旧消费者忽略提示，仍校验完整配置 schema |
| 现有 type 删除/改变必填字段或语义 | 升级该 type 的 data_version |
| 改变封套路由/身份等必填字段语义 | 升级 schema_version |
| 配置响应新增不兼容的必填结构 | 升级 metadata_schema_version，并同步升级平台加载代码及对应 adapter |

破坏性版本升级时生产方不能直接对所有队列发送新格式。由系统部署的镜像/协议支持版本及兼容窗口决定格式；如需要双格式发布，以独立传输 ID 处理，利用 scope/selection_revision 做业务幂等。具体滚动升级兼容窗口待部署规格定稿，不为 Skill 版本管理新增能力登记系统。

## 失败与结果可观测性

| 情况 | 处理 |
|:---|:---|
| 重复/乱序 | source/event_id 去重、权威选择序号核对；不按时钟排序 |
| 用户显式回滚资源 | 新 selection_revision，允许应用所选旧资源版本 |
| Gateway/Control/Storage 暂不可用 | 延迟重试、保留旧配置、周期对账 |
| 配置不兼容或确定无法加载 | 不标记应用成功；报告错误，避免无上限毒消息重投 |
| 目标不存在/授权确定失效 | 明确失败，不冒充 superseded、不扩大权限 |
| 未知 changes 提示 | 忽略提示，拉取与校验完整目标 |
| 未知协议/数据主版本或 type | 隔离观测，不执行、不无限重投 |
| Agent 离线/重启/通知漏投 | 查询当前已确认选择，不读取任意最新资源 |

应用结果观测必须关联 scope、selection_revision、配置引用、稳定 replica_id 与本次实例 incarnation_id，区分 applied/failed/incompatible/superseded。Control 接收观测时只更新对应实例代次与选择的投影，晚到旧实例结果不能覆盖新实例状态。

结果通过 Agent → Gateway → Control 的受控 API 上报，不新增 agent.metadata.update_reported 消息。Gateway 验证并代理，Control 保存应用投影；RabbitMQ ACK 不作为该业务反馈。

## 应用结果上报 API 契约

HTTP 路由为 Agent → `POST /api/agent/v1/metadata-results` → Gateway → Control 的内部结果接口；完整接口契约见 [Agent Runtime spec](agent-runtime.md)。结果内容如下，Gateway 验证归属后代理 Control，不缓存。

```json
{
  "report_id": "report-001",
  "workspace_id": "ws-001",
  "agent_config_id": "cfg-001",
  "selection_revision": 18,
  "target_context_version": "42",
  "replica_id": "0",
  "incarnation_id": "runtime-boot-001",
  "status": "applied",
  "applied_context_version": "42"
}
```

status 为 applied/failed/incompatible/superseded。applied 必须携带匹配目标的 applied_context_version；failed/incompatible 携带稳定 error_code，可带脱敏的 resource_ref，不含凭据、文件正文、原始异常或敏感路径。superseded 表示该选择不再是用户有效目标，不等价于应用成功。

report_id 标识一个稳定结果，重试复用。Control 验证 Agent Configuration、Workspace、Gateway 核验的副本归属及当前运行代次；实例 ID 不能只从正文建立信任。Gateway 结合 Kubernetes 资源身份与 Agent 启动注册核验代次，向 Control 传递受信的实例归属；自定义 Controller 不做实例探测或登记。未知或无法核验实例返回明确错误，不凭请求创建有效实例。

### 幂等与状态规则

- 相同 report_id 与相同内容返回既有接受结果，内容冲突拒绝。
- 投影分区为 workspace/agent_config/selection_revision/replica/incarnation；target_context_version 必须与 Control 对应选择匹配。
- 旧选择或旧运行代次结果不能更新当前视图；晚到结果可作为历史，不能标记当前实例成功。
- 同一分区 applied 后晚到 failed/incompatible 不降低成功；failed/incompatible 可被后续 applied 收敛。Runtime 健康故障使用独立观测，不撤销历史应用事实。
- Agent 保留可重建的应用结果与待上报状态。Control/Gateway 不可用时独立重试上报，不重新安装已应用配置、不回滚成功结果；启动后按实际文件/状态恢复并以上述代次规则重报。

### 用户可见汇总

Control 按当前用户选择与当前参与实例的上报展示：尚有未就绪/未上报实例为更新中；全部目标实例 applied 为全部成功；部分 applied、部分 failed/incompatible 为部分失败。没有成功且全部明确失败显示失败，不能把空实例集合算作全部成功。

新增副本/重启代次需重新确认实际配置，不能继承旧进程结果。成员为当前期望的非 draining 副本；未就绪/未注册者保持 pending，扩容增加成员，完成缩容移除成员，历史结果保留。300秒无明确结果显示超时等待并继续对账，不伪造失败。汇总是可重建投影，具体字段以 [Agent Runtime spec](agent-runtime.md) 为准。

## 实现映射

Control 的选择事实属于 Agent Configuration；跨服务发布位于 biz/workflow，Broker adapter 在基础设施边界。Gateway 实现受控查询代理与平台正文生成，无配置缓存。Agent 消费/加载位于 adapter/biz/workflow，具体 Runtime 负责转换和版本化加载。

- 不可变资源历史/内容引用、完整配置快照与 selection_revision 持久化。
- 固定配置查询/结果上报 IDL与统一配置响应 schema。
- 队列生命周期、延迟重试、隔离、对账周期与滚动兼容窗口。
- Runtime 的旧 Session 隔离及跨实例共享文件发布。
- 多副本用户汇总的目标实例集合、排空/离线计数与等待超时。

上述内容的实现契约已归入 [Agent Runtime spec](agent-runtime.md)，代码和迁移完成状态见 [落地计划](../plan/agent-runtime.md)。本消息契约不代表实现完成。扩展性审查见 [Review](../review/agent-metadata-protocol-extensibility.md)。

## 固定接口与产物

Control 用户 API：`POST /api/v1/workspaces/{workspace_id}/agent-metadata:confirm`、`GET .../agent-metadata/progress`、`PUT .../agent-metadata/artifacts/{checksum}`。确认携带 agent_config_id、expected_version、idempotency_key 与完整资源版本向量；同 key/相同内容返回已有 Release，内容或 CAS 冲突409。

Agent 经 Gateway 使用 `GET /api/agent/v1/runtime-context`、`GET /api/agent/v1/metadata-artifacts/{checksum}` 和 `POST /api/agent/v1/metadata-results`。查询作用域包含 workspace_id/agent_config_id/replica_id/incarnation_id；可指定 selection_revision/target_context_version；`for_session=true` 只用于读取固定历史目标。Authorization Bearer 验证稳定 Agent 委托，`X-Agent-Instance-Credential` 验证当前已登记代次的独立秘密，Gateway 只保存 SHA256 摘要。未知注册或旧代次403，事实源不可用503，不由请求自建注册。

Gateway 到 Control 的内部 API 另外要求既有服务 Token、已验证 acting user，以及 `X-Metadata-Gateway-Credential` 专用代理凭证。它不是 Agent 的 API Token，也不写入元数据正文。成员查询返回 `membership_complete`；缺少原生工作负载完整成员事实时汇总显示 waiting_instances。

metadata_schema_version=1 的配置包含作用域、selection_revision、UUID target_context_version、数字 context_version、agent_type、configured_prompt、platform_prompt、platform_prompt_template_version 及 resources。资源使用 kind/id/version/name/checksum/content_ref/content；当前可确认 Control-owned Prompt、Skill、Memory Store，未知必填资源422或 incompatible。新增提示 kind 不受此资源支持范围限制。MCP/Plugin 资源的实际加载属于对应后续阶段。

Skill 上传正文为 JSON base64 `content`，压缩文件上限8MiB，服务请求体上限默认12MiB；Control 本地 CAS 存储，Catalog content_path 必须为 sha256/checksum。ZIP 必须在根目录包含有效 UTF-8 SKILL.md 和 name/description frontmatter；解包上限32MiB/1024项，拒绝路径逃逸、链接、重复目录名、加密和特殊文件。同一选定集合的 Skill name 不能重复。

Memory 产物保留 id/name/store_type/status、credential_reference_id 与固定 configuration_ref(kind/id/version)，校验后写 memory/{id}.json。任意 provider configuration 不直接复制到快照，秘密值不进入 release、Prompt 或消息；按引用取配置/执行凭据由对应 Memory/MCP 接入负责。applied 表示该阶段完整配置产物已准备，不代表外部 Memory 服务健康。

Agent 使用 `.metadata/{config}/bundles/{target}/{snapshot_hash}` 发布完整目录，checksums.json 校验文件集及 hash；实例 marker 位于 instances/{replica}/{incarnation}/application.json。Configured/Platform 正文分别落盘；Runtime 的格式转换在 Agent 侧完成。结果 error_code 限定稳定小写标识，禁止原始异常/路径；相同 report_id 内容冲突409，applied 后的晚到失败不撤销成功。

部署配置由 typed `agent_metadata` 与 Python `metadata` 定义；默认关闭。Control artifact_root 需要持久卷；多 Control 实例需共享同一对象根，Gateway 不保存 Control 配置。已启用并创建固定元数据 Session 后不能通过关闭加载开关让这些 Session 退回非版本目录。实例注册及凭证签发在 P3 接入，原生期望副本集合在 P5 接入；P2 已验证读取已登记实例事实并拒绝未经登记的启动。
