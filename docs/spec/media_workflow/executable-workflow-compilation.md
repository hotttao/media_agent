# Executable Workflow 编译实现规格

## 审批的执行重试周期

Review Delivery使用独立ApprovalTaskEngine端口，`CompleteApprovalTask(workflowID, frozenTaskID, output)`直接映射Conductor `POST /api/tasks`，不按引用名再次GET/解析；旧CompleteTask-by-reference不作为新Outbox投递路径。没有精确端口则释放当前claim并保留固定故障码，不静默回退。空实例或Task ID拒绝；该改动不向客户端开放内部Task提交权限。

migration021给step_run_reviews增加可空非负accepted_retry_count；已有记录保持null且不改摘要。新DecideReview在读取Job后冻结retry_count，并在AcceptReview的Job→Step锁内核对；同键原决定重放先于当前周期检查。ReviewDecision存在该字段时将标记对象`{"accepted_retry_count":N}`追加至原摘要数组，null沿用旧格式；字段不可修改，带周期事实的迁移不得降级删除。

后台发现新Review周期不等于当前Job时，只标记failed/execution_retried，不调用引擎。投影以Engine.Get之前冻结的Job周期比较Review：更早周期忽略、更晚周期拒绝，同周期继续数字generation与精确HUMAN检查。读取到旧null周期不能当成0或回填；重试后同轮必须由已完成且精确匹配的HUMAN证明，否则保留安全拒绝。未知旧周期不参与自动恢复推断。该契约不改变公开IDL、产物结果键或权限边界，不替代可靠Retry命令及rerun与本地计数之间的窗口恢复。

## Conductor 注册一致性校验

发布在保存 Compilation、注册 Task Definition 前，通过只读 metadata GET 查询编译结果对应的精确 name/version；404 才表示未注册，其他查询错误拒绝继续。已存在版本按 Compiler 的规范 JSON 比较摘要；冲突不能覆盖。额外字段只允许已验证的 Conductor 默认值、空扩展及审计时间；非默认的异步、可选、延迟、变量/输入模板和关闭 schema 校验等配置不得因结构解码忽略。任务树中的循环、并行分支、条件分支也要递归检查。

注册 Workflow 时再次检查精确版本；已有一致结果不写入，缺失才发送一次 PUT，之后必须读回并验证。PUT 响应不明确但读回一致可确认成功；查不到、查询不可用或内容变化均不能标记 registered，也不自动重发未知写入。错误只保留稳定码，不保存上游 body、URL 或业务载荷。

此过程不提供上游 CAS：并发外部作者可在两次查询之间改写 metadata，完整发布仍须冻结的本地 Compilation、可靠注册意图和所有注册者的版本隔离。Workflow metadata 校验不能替代 Task Definition 的共享契约保护。

Task Definition 注册先校验同批名称无矛盾，再逐名GET。仅404代表缺失；查询失败拒绝继续。既有名称必须匹配被冻结的description、重试次数/策略/间隔、超时与response timeout；未知非空执行扩展（inputTemplate、schema、并发/限流、隔离等）拒绝。允许Conductor审计字段与经锁定源码/实际服务确认的默认字段；请求省略值按引擎默认规范化，服务显式零值不能被当成“省略”。taskStatusListenerEnabled必须为默认true，backoffScaleFactor和rateLimitFrequencyInSeconds必须为默认1。

整批预检通过后，仅一次POST创建缺失子集，不PUT更新同名任务。无论POST是否返回成功，都以逐名GET验证完整契约；提交成功但响应丢失可通过原定义对账收口，未确认则保留失败，不盲目重写。TaskDefinition不同内容必须使用新Capability Version对应的新task name；同名旧定义不会原位修复。这仍不是上游CAS或发布后台恢复，外部注册者并发修改窗口需另行协调。

本地Compilation以 `(workflow_id,workflow_version)` 首次插入冻结。重复Save只有ID、Conductor名称/版本、DefinitionHash、IR及Definition JSON内容均一致才成功；JSONB比较忽略对象键顺序/空白，不忽略数组顺序或值类型。重放不重置状态、创建时间、注册时间或错误记录。不同冻结内容返回Compilation冲突，必须发布新版本。MarkRegistered保持首次注册时间；已registered后晚到MarkFailed仅保留成功事实，缺失ID不能假报标记成功。migration020在数据库层限制冻结字段修改及registered状态/时间/错误的回退，未改写既有编译数据；本地约束不等同于发布Outbox、TaskDefinition冻结或上游metadata的CAS。

## 联调发布兼容性

Capability Task input 在 Invocation Mapping 之外固定包含 `job_id=${workflow.input.job_id}`、原始 `step_key`、`capability_key` 和 `capability_version`。Worker Gateway 使用这些字段加载并核对 Job、StepRun 和执行实例；不从 task reference 名字推断缺失归属。缺字段的历史联调样本先经 `CancelJob` 结束，再通过现有迁移工具创建/发布新样本，成功后归档旧 Workflow；不修改旧 Compilation 或 Conductor 执行快照。

真实 Worker 单样本 HTTP 门禁位于 `media-cli/e2e/workflow_personal_worker_fixture_test.go`，显式开启才能提交一次。设置 `MEDIA_E2E_SAMPLE_JOB_ID` 时只查询/确认已有执行，绝不再次提交。测试使用 User API Token 访问统一网关，外部独立运行的 media_worker 使用 Worker API Token；内部渲染/收口任务仍由 media_workflow 执行。多候选审批由测试模拟用户调用确认接口，不能由 Worker 自批。普通单测不消费测试环境队列。

## 范围

### 本轮功能收口（2026-10-04）

原生 Submit 对 Product 主图、Virtual IP 各图片、Material 与 Asset URL 校验 `MEDIA_STORAGE_PUBLIC_ENDPOINT`、`/media/materials/<organization_id>/` 路径和当前可读性，持久化为 `object_path:<path>`。复制输入后转换，不修改调用者对象。纯文本 Material 的空 URL 不解析。Personal Worker Gateway 在实际交付和租约重取时解析稳定引用，Job、Conductor 和 Dispatch 均不因重签改写；原生内容中的 `gen_` 普通字符串不能作为历史 Generated Asset 查询。对象不可用、外企业对象或签发地址即将过期时拒绝交付。

Managed Runtime 复用同一对象解析与稳定 Dispatch 快照，企业来自 Job，不能从 Conductor Task 的任意字段派生。内部服务端解析使用 Materials 服务凭据，不为 Managed Worker 新增 API Token。

交付时，服务端用已知输入引用和已解析的媒体引用替换渲染结果 `prompt` 中的引用，不在客户端重新渲染模板，也不解析提示词中的任意外部 URL。上游产物的链接只有匹配受控存储来源及 Job 企业命名空间时才重新签发。Retry 的输入覆盖按 Job 对应 Compilation/IR 和冻结 Template Version 派生的 Schema 校验，媒体重新验证后冻结；Retry History 也保存冻结输入，不记录临时签名。错误发生在重跑命令和状态修改之前。

先完成普通 User 的新建执行链路，暂缓历史数据核对、迁移及 UI 实现，不扩展 Agent 生命周期。SubmitJob 必须提供目标 Workspace；服务端通过 Control 校验当前用户的 Workspace 访问及 `use_workflow` 权限，并派生 Job 企业归属。请求的企业选择与 API Token 绑定企业均不能越过 Workspace 的真实企业；未绑定企业的浏览器身份也不能省略 Workspace。Personal Worker 交付前必须匹配 Job owner、企业和执行实例，企业归属为空或不匹配时不创建 Dispatch。

本规格定义 `media_workflow` 从业务 Workflow 发布到 Conductor Definition 的两阶段编译，以及内部模板渲染 Worker。FlowGram 文档、业务 Workflow DSL、Executable Workflow IR 和 Conductor Definition 必须分层，不互相充当事实源。

## 核心类型

```go
type ExecutableWorkflowIR struct {
    SchemaVersion int
    WorkflowID string
    WorkflowVersion int
    OutputStepKey string
    Steps []ExecutableStepIR
}

type ExecutableStepIR struct {
    Key string
    DependsOn []string
    Template TemplateIR
    Context map[string]ValueRef
    Capability CapabilityIR
    Invocation map[string]Expression
    OutputAssetKind string
    ApprovalPolicy string
    ReusePolicy string
}
```

Expression 初期只允许：

- `render_template`：结果为 string；
- `asset_ref(slot)`：结果为 ImageRef/VideoRef 等资源引用；
- `list(expressions...)`：保持声明顺序，结果为数组。

禁止任意脚本、反射和 Worker 端按字段名猜测。

## Semantic Compiler

输入为冻结的 Workflow、Template Version 和 Execution Capability Version。按 Slot Binding 解析 context，检查 DAG、引用、Template Slot、Invocation Mapping required 覆盖及类型兼容，输出确定性的 `ExecutableWorkflowIR`。相同输入必须产生相同 canonical JSON。

`WORKFLOW_INPUT` Binding 必须显式保存入口名，支持 `full_body -> person`，不得再隐式使用 `to_input` 作为入口名。

## Conductor Backend Compiler

只接受 Executable Workflow IR。每个 Step 至少降低为：

1. `media_render_template_v1`：输入不可变 template body 与动态 context Map；
2. Capability SIMPLE Task：`prompt` 引用 Renderer 输出，其他参数由 Invocation Mapping 直接降低；
3. 候选持久化、审批/重生成及唯一 `output_resource_id` 收口。

预期单候选且策略为 `when_multiple_candidates` 时，Capability 后仍编译运行期 `SWITCH`，依据实际 `candidates.length` 决定是否进入 HUMAN；不能因为预期为 1 而删除多候选审批路径。预期多候选或 `always` 则直接进入 HUMAN。Finalize 在审批分支完成后使用唯一选中候选；自动单候选分支允许选中候选为 null，由 Finalize 收口该唯一候选。旧 Compilation/Definition 不原位覆盖，修复后的编译结果通过新版本发布生效。

保持现有拓扑波次策略。Definition 不包含 Job ID、密钥、Token、二进制或临时 URL。Definition canonical JSON 用于 SHA-256 hash。

每个 Step 编译为 `DO_WHILE(Renderer -> Capability -> HUMAN 或动态审批 SWITCH) -> Finalize`。循环条件仅在本轮 HUMAN 输出 `decision=regenerate` 时成立，自动单候选没有 HUMAN 则结束循环。循环内引用采用原始 Step Key 的十六进制编码，形如 `step_<hex-key>_invoke`；只允许 Conductor 在尾部添加 `__<positive-iteration>`，避免与其循环引用解析冲突。Finalize 保留循环外稳定引用供后继使用。

对账按 Step Key 选最新轮任务，不能依赖任务数组顺序；完成旧轮的审批输出不再触发 needs_confirm，旧失败不覆盖新轮。确认与重新生成先读取真实执行，找到该节点最新轮 `IN_PROGRESS` 的 HUMAN 精确引用；确认输出 `decision=accept` 和选中候选，重新生成输出 `decision=regenerate`。旧无循环定义只能确认，重新生成返回明确不支持错误，不尝试终止整个 Workflow。

## Renderer Worker

Worker 属于 `media_workflow`，与 API 共用代码库但以独立进程部署和扩缩容。Task Definition：

```json
{
  "name": "media_render_template_v1",
  "input": {"template": {"id": "", "version": 1, "body": ""}, "context": {}},
  "output": {"prompt": "", "prompt_digest": "sha256"}
}
```

纯 Renderer 位于 Template Domain；Conductor poll/ack 位于基础设施适配层，Worker workflow 负责二者编排。Renderer 不查询 `media_materials`、不调用 Provider、不决定 Capability 参数顺序。同一输入必须确定性输出；缺少字段、模板语法错误或不允许的字段访问返回稳定错误码。

新编译 Renderer 输入增加 `feedback_mode=last_regeneration_v1`、`generation=${<loop>.output.iteration}`、`previous_generation=${<approval>.iteration}`、`previous_review=${<approval>.output}`。Renderer 在本轮 HUMAN 创建之前调度，这两项审批引用应取紧邻前轮；必须在真实 Conductor 验证，不依靠静态引用猜测。首轮 generation=1，审批引用为空；后续要求 previous_generation=generation-1 且 previous_review.decision=regenerate，否则拒绝。不能复用其他节点、非相邻轮或 accept 的反馈。首轮基础提示词不变；新轮重新渲染冻结模板，反馈非空时追加 `\n\n用户重新生成反馈（本轮）：\n<feedback>`。原文最多 8192 字节，不作为模板执行，不累计较早反馈，完整结果参与 prompt_digest。旧 Renderer 输入没有 feedback mode 时保持原行为。

## Worker Execution Target

Capability Version 必须冻结 `execution_target=personal_worker|managed_worker` 和可选 `managed_pool_key`，Executable Workflow IR 与 Conductor `taskToDomain` 均以该冻结值为准。完整认证、双 Target 调度和迁移契约见 [API Token 与 Worker Execution Target 实现规格](../api-token-worker-execution.md)。

## 客户端 Media Worker

Mock Profile 的旧样本资源服务根地址由 `MEDIA_WORKER_FIXTURE_ASSETS_URL` / `--fixture-assets-url` 配置，默认与素材迁移工具一致。只能是无凭据、无 query/fragment 的 HTTP(S) 地址；结果文件路径与 mock provenance 冻结不变，仅支持内置 Capability Version 1。该测试配置不替代生产对象上传，不会回写已完成 Job 的历史 URL。

独立 `media_worker` 可执行程序只接受统一 Gateway URL、绑定 Personal Worker Actor 的 opaque API Token、`worker_instance_id`、启用的 Personal Capability Version 列表和 profile；不得配置 Conductor URL。它经 `/api/worker/v1/*` 注册、长轮询、心跳并回报结果。生产 profile 调本地 Provider/CLI，`media_ai_fixture` profile 按 Capability Version 返回冻结样本并附带 provenance。它不能提交任意 task type、Managed Capability，也不得领取 `media_render_template_v1` 或 `media_finalize_step_v1`。

`media_workflow` 提供独立 internal-worker 入口执行 Renderer/finalizer。API 进程负责发布、注册与启动 Workflow，不在 HTTP goroutine 中执行 task polling。

### Worker Gateway API

所有路由先经 Traefik + Oathkeeper/Edge Auth。Edge 调用 `media_auth.VerifyAPIToken`，为 opaque Token 对应 Principal 签发 audience=`media-workflow`、TTL 1 至 5 分钟的 Internal Identity JWT。委托身份使用 `sub=User` 与 `act.sub=Worker`；Worker 路由不要求 Kratos Cookie。

```text
POST /api/worker/v1/registrations
POST /api/worker/v1/tasks:poll
POST /api/worker/v1/tasks/{dispatch_id}:heartbeat
POST /api/worker/v1/tasks/{dispatch_id}:complete
POST /api/worker/v1/tasks/{dispatch_id}:fail
```

JWT `sub` 为业务 User ID，`act.sub` 为稳定 Personal Worker Actor，并包含 `organization_id`、`credential_id`、`scope`、`auth_source`、`jti`。有效权限为 Token scope、当前 Organization Membership/Entitlement、Job owner、Personal Capability allowlist 的交集。Gateway 必须移除客户端伪造的 Authorization/identity header，原始 Token 不进入业务服务和日志。

Worker 只声明 `(capability_key, version, concurrency)`。服务端查询 `execution_capabilities` 得到 taskDef 并要求 `execution_target=personal_worker`，排除 Managed 与 Internal Execution Task。启动 Conductor Workflow 必须使用 `taskToDomain`：Personal 从 Job User 派生，Managed 从受控 Pool 派生；不接受客户端 domain，不允许跨 Target 或 `NO_DOMAIN` fallback。

MVP 可以为一个 Worker 的每个 Capability 建立一个受取消控制的 long-poll；后续可替换为服务端共享 Poll Multiplexer，只为活跃 `(execution_domain, taskDef)` 维护 poller，外部 API 不变。

### Dispatch、租约与结果收据

新增：

- `worker_dispatches`：保存内部 conductor_task/workflow ID、job/step_run、attempt、Capability、user/org/credential/worker instance、status 和 lease；
- `worker_result_inbox`：以 `(dispatch_id, attempt, operation)` 唯一，保存校验后的 result、digest、receipt 与 Conductor 提交状态。

Conductor poll 成功后必须先提交 Dispatch 事务再返回客户端。响应丢失时下次 poll 返回既有有效 Dispatch；持久化前崩溃则由 Conductor response timeout 回收。Heartbeat 延长本地 lease，并用 Outbox/Adapter 更新 Conductor callback/lease。旧 attempt、过期 lease、取消 Job 的结果分别返回 409/410，不覆盖当前状态。

Completion/Failure 验证 Principal 绑定、attempt、lease、输出 schema 与 Object Path 后，事务写 Inbox 和 `result_received`，立即返回 `202 + receipt_id`。后台 Adapter 重试 `POST /api/tasks`；重复请求返回原 receipt，不要求客户端因 Conductor 暂时失败重新执行 Provider。

### 对象传输

Definition 与持久任务只保存 Object Path。领取时生成短时 presigned GET/PUT URL；Worker 上传后回传 Object Path、checksum、content_type、size 和 provenance，不能提交任意外部 URL。Provider URL 必须先摄取到受控存储。Finalize 校验对象后，按原生生成结果登记规格，以稳定 StepRun ID 与 `result_key=SHA256(JSON([ConductorWorkflowID,StepRunID,当前轮唯一成功CapabilityTaskID]))` 登记该轮独立 Generated Asset。同轮重放原ID，新成功能力任务新ID，StepRun指向最新接受结果，旧产物保留。Worker Dispatch attempt 与 Finalize重试任务ID不是产物轮次身份；完整约束见 `docs/spec/generation-result-idempotency.md`。

## 持久化与版本

Template 与 Template Version 增加 `capability_key`、`capability_version` 和 `invocation_mapping jsonb`。新增版本化 `execution_capabilities`、`worker_dispatches` 和 `worker_result_inbox`。`workflow_compilations` 保存源版本、Conductor name/version、definition_hash、状态与错误；运行时不从该表加载 IR。已发布 Capability Version 不可修改，archived 只阻止新引用，不能删除在途任务仍使用的 taskDef。

## Harness 分层映射

- API 契约新增 `idl/media_workflow/worker.proto`，通过 `hz` 生成 model、client、router 和 handler 骨架；不手改生成文件。
- Worker handler 保持请求绑定和响应映射，认证技术上下文由 `media_workflow/biz/middleware` 提取；Organization/Job/Capability 权限决策放在 policy/service。
- Job、StepRun、WorkerDispatch 与 Result Receipt 的实体、不变量和仓储接口落在现有 `media_workflow/biz/domain/job`；单聚合命令由 `biz/service/job` 控制本地事务。
- Conductor poll/update、Dispatch/Inbox、Outbox 与对象 presign 的跨事务流程落在 `media_workflow/biz/workflow/conductor`，外部调用期间不得持有数据库事务。
- 数据库变更先添加 `media_workflow/migrations/*.up.sql/.down.sql`，行模型与 repo 落在 `biz/dal/model`、`biz/dal/repo`，生成查询只能通过 GORM Gen 更新。
- `media_auth` introspection 通过生成的 typed client 装配到 `media_workflow/biz/shared/client`；Conductor 与 S3 使用各自基础设施 adapter，不把 SDK 类型泄漏进 domain/service。
- Traefik 只配置路由与 ForwardAuth，opaque Token 内省和 Internal Identity JWT 签发由 Oathkeeper/Edge Auth 配置或适配器承担。

## 测试

- Semantic Compiler：required 参数缺失、未知 Slot、类型错误、顺序稳定、上游 Step 引用、确定性 JSON。
- Renderer：动态 context、嵌套字段、缺失字段、非法模板、确定性 digest。
- Backend Compiler：Renderer 自动插入、Capability 参数引用正确、images 顺序、拓扑波次、hash 稳定、无敏感字段。
- Worker Gateway：Service Token exchange、无 Cookie、scope/org/job/capability 拒绝、内部 task 拒绝、按 User Domain 领取、无 `NO_DOMAIN` fallback。
- Dispatch：响应丢失恢复、heartbeat/lease 对齐、撤销 Token、取消、旧 attempt、重复 complete、Conductor update 暂时失败和 Adapter 重试。
- 对象：即时 presign、上传摘要校验、任意 URL 拒绝、Finalize 幂等登记。
- 集成：注册 Definition 后，Renderer Worker 输出 prompt，Capability Worker 经 Gateway 收到固定 `prompt + images[]`，客户端进程不具备 Conductor 网络访问。
- E2E：迁移 `media_ai/scripts/workflow-cli` 的七阶段视频流程；只导入一个 fixture execution，启动真实 Gateway/Auth/Conductor/internal worker 与 `media_worker --profile media_ai_fixture`，从 UI/API SubmitJob 并验证最终视频结果。用户确认前不迁移其余旧数据。

## Job 输入表单契约

- `InputSchemaEntry` 返回 `name/resource_type/asset_kind/required`；`asset_kind` 对 MATERIAL、ASSET 必填，对 PRODUCT、VIRTUAL_IP 可空。
- Workflow Validator 拒绝同名 MATERIAL/ASSET input 绑定不同 asset kind。
- 独立前端读取已发布 Workflow 的 scope、version 与 Input Schema，按 Organization/Scope/Asset Kind 查询候选并组装 JobInput；不向业务用户暴露 JSON 编辑器。
- MATERIAL 使用 object path 的可访问 URL 与文本内容形成 bundle；Product、Virtual IP 使用稳定业务字段形成快照。提交显式携带当前 Workflow version。

## 人工审批运行时契约

### 用户 CLI

`media_workflow job get --id <job>` 返回 Job 状态和节点摘要；`step-run get --id <step>` 返回节点状态、候选数量、选中数组位置、输出资源 ID 及最近 Review 回执。两者不打印候选 URL、动态输入或渲染提示词。

`step-run retry --id <failed_step> --reason <optional_text>` 使用既有公开 Retry API，成功须为 200 且响应 StepRun ID 与请求匹配；只打印安全摘要（含 retry_count）。不接受输入覆盖，不冒充持久审批决定，也不自动重试 HTTP 请求。错误或响应不明确时提示先查询 Job/StepRun；当前 Retry API 尚无持久幂等命令，不能把重复发送视为安全恢复。与 HTTP 重试门禁分开验证独立 CLI 进程完整流程。

`step-run confirm --id <step> --selected-index <array-position> --idempotency-key <stable-key>` 与 `step-run regenerate --id <step> --feedback <optional-text> --idempotency-key <stable-key>` 调用公开网关 API，成功须为 202 且有 Review 回执。显式稳定键必填，同一决定网络重试复用；新决定使用新键。凭据从既有 API Token 配置取得，禁止请求身份覆盖或输出上游原始错误。CLI 与 Go HTTP 测试分别验收，查询/终态重放不能代替新执行门禁。

### 持久化决定与恢复

SubmitJobReq 的 `idempotency_key` 位于字段 7，保持可选；Handler 仅绑定可信作用域并交给 Submit，摘要冲突固定返回 409，不返回原始请求或上游错误。`media_workflow submit-job --idempotency-key <stable-key>` 透传该键；CLI 仍通过当前授权的库资源读取/解析组装输入并确定发布版本，资源内容或发布版本改变可能导致冲突，不提供离线冻结请求重放。CLI 不自动重试；响应不明确时提示查询执行或复用同键，不能自动产生新键。生成 Hertz Client 在错误时可能丢弃 rawResponse 并将 status/body 包成 JSON 错误，CLI 只检查 status_code，不展示 body。

公开 Submit 可选 idempotency_key；省略保持每次新提交，提供时要求 1–160 个可见 ASCII 字符（不含空格）。migration 019 `job_submissions` 持有 `(submitted_by, organization_id, workspace_id, idempotency_key)` 主键、唯一 Job 外键 RESTRICT、64 位小写十六进制 request_digest、created_at。`SubmissionRepo.GetSubmission(ctx,key,lock)` 在首次接受事务内用完整作用域 SHA-256 的 64 位值取得事务级 advisory lock 后二次查询；非事务 lock/save 拒绝。Save 验证 Job owner/企业/Workspace、native 与初次启动记录已存在。摘要由 Workflow ID、请求版本（含 0）、规范化稳定媒体输入产生，不能持久化签名 URL；同键不同参数返回 409，同键同参数返回原 Job 当前状态。授权每次重验，重放先于当前 Workflow/媒体读取；同键并发必须只保存一个完整聚合。HTTP/CLI 接线与真实并发另行验收。

普通 Submit 启动落库新增 migration 017 `execution_starts`，由 Execution Job Repository 持有。`ExecutionStart` 冻结 Job ID、引擎 Definition 名称/版本、Compilation hash、correlation=Job ID、workflow_input 与 task_domains JSON，以及规范化 JSON 摘要。EnqueueStart 必须使用保存 Job/StepRun 的调用方事务，只有无实例 native pending Job 可写，Compilation hash 同时绑定至 Job。

`ExecutionStartEngine.FindStartedExecution(ctx,ExecutionStart)`使用完整冻结命令。correlated(includeClosed=true)只定位唯一候选，随后GET该实例，核对workflowId/correlationId、workflowDefinition.name/version、完整input及taskToDomain的规范JSON。未找到返回空；冲突/缺字段/读失败返回固定安全错误。已发送首次Start响应丢失也使用同一精确校验；uncertain恢复与取消均不得只凭correlation绑定/终止。当前普通启动入口未实现RETURN_EXISTING，仍禁止uncertain补发，发送前崩溃无法自动取得进展；引擎级幂等方案需要独立验证与部署决策。

ClaimStart 使用 SKIP LOCKED 和新 UUID token，queued/uncertain 均可领取；MarkStartSent 只允许有效 token/两分钟内租约且 Job 仍 pending 的 queued→uncertain。事务外发送一次 Start 后，FinishStart 同事务写 Job running/实例及命令 started，必须匹配冻结摘要、token、租约；ReleaseStartClaim 只保存固定码和十秒退避，不改变 queued/uncertain。回滚与降级保护不得丢失启动事实。当前引擎不保证启动去重，后台恢复 uncertain 只查询原实例，不再发送。取消与实例终止接线、公开 Submit 幂等、进程重启以及实际部署单流程另行验收；仓储实现不等于功能已上线。

迁移 018 扩展启动取消状态：cancel_pending 必须有实例，cancelled 可有已清理实例。`CancelInitialStart` 锁 Job 后锁启动记录，同事务取消 Job；queued 清除 claim 转 cancelled，started 清除 claim 转 cancel_pending，uncertain 保留状态供只读恢复。重复 cancelled Job 返回原事实，其他终态拒绝取消；无初次启动记录返回未接管，由原有路径处理。Claim/Release 包含 cancel_pending。`FinishCancelledStart` 只允许 cancelled Job、uncertain/cancel_pending、有效 claim/摘要及匹配实例，记录 cancelled 清除租约，不修改 Job。后台 uncertain 只查询原实例；cancel_pending 使用已冻结实例 ID。先 Get 引擎状态，非终态才 Terminate，再 Get 确认终态才收口；终止响应丢失保留义务，下一轮先读，不盲目再次终止。取消实例查询无结果不标完成。在线接线与真实故障恢复需单独验收。

失败 Step Retry 从最新生成轮 Renderer 开始，前提是本地 Job 与 Conductor 均为失败终态且未超过次数上限。Adapter 保留任务 `seq` 的缺失语义（可空整数，不以默认 0 冒充已知顺序）。rerun 前检查其他业务节点：按 seq 会被删除、保留但未完成会被重开、缺失顺序或未知 SIMPLE/HUMAN 均拒绝直接重跑；拒绝不能变更 StepRun/Job/Retry History 或调用 rerun。已完成且位于目标之前的节点保持原样。

这是并行恢复开发期间的防误执行保护，不代表支持全部 DAG Retry。完整恢复需从冻结 Compilation/IR 派生受影响集合，持久化幂等重试意图和保留的产物引用/反馈，由后台协调引擎及本地状态；不得重跑无关已完成 Capability。不能将旧 historical restore 入口直接开放给原生 Job，或更换用户/Workspace 实现恢复。

冻结图分析由`executable.PlanStepRetry`纯函数提供：仅当前SchemaVersion、有效源身份/版本、唯一Step key、完整依赖、唯一输出sink且无环的IR可参与。`DependsOn`与Context内StepOutput引用集合必须相等，不能隐藏或漏声明边；返回按拓扑顺序的Affected（目标及所有传递下游）、Preserved（其余节点）以及Boundary（Preserved输出进入Affected的每个业务slot）。存储数组顺序不改变结果，不根据Conductor seq决定业务依赖。Preserved不是“已完成”的声明；恢复协调器仍必须验证运行产物回执，不得凭此函数执行模型或恢复旁支。

标准Conductor Step Retry在任何engine rerun前读取Job原版本的registered Compilation，核对源ID/version/hash、严格解码IR、核对IR身份并计算图分区；StepRun集合必须与冻结节点一一对应。不能查询可编辑Workflow作为重置依据。已有直接rerun隔离保护不放宽；允许rerun后按Affected清空目标和传递下游的候选/选中项/产物指针并置blocked，Preserved不写。本地写入错误返回安全失败与查询原执行指引，不能忽略并报告成功。这仍没有消除引擎提交与本地保存之间的窗口，也不是持久Retry意图实现。

原生Step/Job Retry的引擎请求在本地事务外；允许请求成功后，本地提交使用caller Tx及Job→Step锁序，重读并核对失败状态、原实例、Compilation hash和原Job retry_count；Step另核对原节点身份/计数及failed/skipped。目标/下游或整单节点的候选、选择、产物指针、旧claim及执行时间清空/blocked状态与Retry History、Job retry_count/running在同一事务保存，失败全部回滚。整单覆盖输入也在该事务保存。取消或新周期发生后晚到本地提交拒绝；配置了原生Review锁端口却没有TxRunner时，网络请求前拒绝。Job事务必须结束后才能继续外部交互。这个事务仅保证本地原子性，不提供外部rerun幂等或崩溃补发。

状态投影在调用 Engine.Get 前捕获 Job ID、ConductorWorkflowID、RetryCount 的值副本；本地 Job→Step 锁内重读当前聚合，不匹配则停止该次投影。外部产物保存前检查一次，保存后本地 Output/Status 事务内再检查；不能在数据库事务中请求引擎或素材服务。校验发现过期后不再调用素材服务或改写节点、审批、终态，Get/List 返回重新读取的当前持久状态，不向用户暴露过期快照错误。若 Retry 在外部保存期间发生，只阻止后续本地收口，不能撤销已经发生的外部副作用。该校验只识别已经持久化的 Retry 边界，不能代替 Retry 意图与引擎的可靠协调，也不能证明重试 POST 后、本地次数增加前的窗口已解决。

`POST /api/v1/step-runs/:step_run_id/confirm` 接收 `selected_index`（候选数组位置）和可选 `idempotency_key`；`regenerate` 接收可选 `idempotency_key`、`feedback`。新调用方每次业务决定生成稳定键（不超过 160 字节），网络重试复用，不同决定或生成轮换新键。显式同键同决定在任务终态后仍返回原回执；不同内容 409。无键兼容只对当前精确 HUMAN 去重，不保证跨轮重放。反馈最多 8192 字节，冻结原文并参与幂等比较；HUMAN input 必须声明 `feedback_mode=last_regeneration_v1` 才接受非空反馈。旧定义非空反馈 400，无反馈重新生成仍兼容。

成功返回 202 StepRun，增加 `last_review_id` 与 `last_review_status=pending|applied|failed`；这是最近决定的回执，不是当前轮待审批标志。当前权限拒绝 403、授权装配不可用 503；身份从网关可信上下文获取。后台每秒领取一条命令，单次投递超时 20 秒，临时错误退避十秒、租约两分钟回收。服务重启不删除 pending 命令。投影在本地事务内锁 Job→StepRun 后复核精确任务 Review，Job waiting 状态在 Job 锁内按步骤重新计算，外部查询不得占用该事务。

`media_workflow.step_run_reviews` 保存 Job/StepRun、精确 Conductor Workflow/Task ID 与引用、generation、decided_by、idempotency_key、decision、候选快照、冻结 output JSON、request digest 和 pending/applied/failed 状态。同轮唯一键为 `(step_run_id, conductor_task_id)`，命令唯一键为 `(step_run_id, idempotency_key)`。同键同内容返回原决定，同键异内容及同轮另一键拒绝。

应用层 TxRunner 开启本地事务；Repository 使用传入事务锁定 Job→StepRun，校验 owner、执行实例、need_confirm 与候选快照，再插入 Review 并把步骤改为已接收决定的 claimed 状态。命令领取使用单条 `UPDATE ... FOR UPDATE SKIP LOCKED ... RETURNING`，过期阈值两分钟，产生新 claim token。完成/失败/释放都必须匹配 token；重放不能重写原 payload，失败原因只保存稳定码，不存完整上游错误。Review pending 时投影不能重新暴露该轮为可确认。

投递过程不持有数据库事务。先查询精确任务，COMPLETED 且输出匹配则只补记 applied；RUNNING/HUMAN IN_PROGRESS 才提交。任务替换、取消或相反决定返回 failed；暂时错误保留 pending，退避后重试。数据库回滚须同时撤销 Review 与步骤状态；迁移回滚不得删除已有 Review 审计。

- Semantic Compiler 在 IR 中冻结 `candidate_count` 与 `approval_required`；Template 多候选/`needs_confirm` 或 Step `approval_policy=always` 均要求审批。
- Backend Compiler 在 Capability task 后插入 Conductor `HUMAN` task。`media_worker` 只返回 candidates，不消费 HUMAN、render_template 或 finalize 内部任务。
- media_workflow 对账 HUMAN task input，投影 `StepRun=need_confirm`；用户确认后以 `selected_candidate` 完成 HUMAN task，Finalize 形成唯一输出并登记 Generated Asset。
- 用户选择重新生成时，完成当前循环 HUMAN 的 `regenerate` 决定；下一轮独立生成，旧轮结果只保留审计，不取消旁路、不调用非终态 rerun，也不把重新生成误当失败重试。
