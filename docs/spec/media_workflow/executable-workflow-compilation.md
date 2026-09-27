# Executable Workflow 编译实现规格

## 范围

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

保持现有拓扑波次策略。Definition 不包含 Job ID、密钥、Token、二进制或临时 URL。Definition canonical JSON 用于 SHA-256 hash。

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

## 客户端 Media Worker

独立 `media_worker` 可执行程序只接受统一 Gateway URL、opaque Service Token、`worker_instance_id`、启用的 Capability Version 列表和 profile；不得配置 Conductor URL。它经 `/api/worker/v1/*` 注册、长轮询、心跳并回报结果。生产 profile 调本地 Provider/CLI，`media_ai_fixture` profile 按 Capability Version 返回冻结样本并附带 provenance。它不能提交任意 task type，也不得领取 `media_render_template_v1` 或 `media_finalize_step_v1`。

`media_workflow` 提供独立 internal-worker 入口执行 Renderer/finalizer。API 进程负责发布、注册与启动 Workflow，不在 HTTP goroutine 中执行 task polling。

### Worker Gateway API

所有路由先经 Traefik + Oathkeeper/Edge Auth。Edge 调用 `media_auth.IntrospectServiceToken`，为 opaque Token 对应 Principal 签发 audience=`media-workflow`、TTL 1 至 5 分钟的 Internal Identity JWT。Worker 路由不要求 Kratos Cookie。

```text
POST /api/worker/v1/registrations
POST /api/worker/v1/tasks:poll
POST /api/worker/v1/tasks/{dispatch_id}:heartbeat
POST /api/worker/v1/tasks/{dispatch_id}:complete
POST /api/worker/v1/tasks/{dispatch_id}:fail
```

JWT `sub` 为业务 User ID，并包含 `organization_id`、`credential_id`、`scope`、`auth_source`、`jti`。有效权限为 Token scope、当前 Organization Membership/Entitlement、Job owner 或显式 Worker Pool 授权、Capability allowlist 的交集。Gateway 必须移除客户端伪造的 Authorization/identity header，原始 Token 不进入业务服务和日志。

Worker 只声明 `(capability_key, version, concurrency)`。服务端查询 `execution_capabilities` 得到 taskDef，排除 Internal Execution Task，并根据 Job `submitted_by` 派生 Execution Domain。启动 Conductor Workflow 必须使用 `taskToDomain`；MVP 按 User 隔离，不接受客户端 domain，不允许 `NO_DOMAIN` fallback。

MVP 可以为一个 Worker 的每个 Capability 建立一个受取消控制的 long-poll；后续可替换为服务端共享 Poll Multiplexer，只为活跃 `(execution_domain, taskDef)` 维护 poller，外部 API 不变。

### Dispatch、租约与结果收据

新增：

- `worker_dispatches`：保存内部 conductor_task/workflow ID、job/step_run、attempt、Capability、user/org/credential/worker instance、status 和 lease；
- `worker_result_inbox`：以 `(dispatch_id, attempt, operation)` 唯一，保存校验后的 result、digest、receipt 与 Conductor 提交状态。

Conductor poll 成功后必须先提交 Dispatch 事务再返回客户端。响应丢失时下次 poll 返回既有有效 Dispatch；持久化前崩溃则由 Conductor response timeout 回收。Heartbeat 延长本地 lease，并用 Outbox/Adapter 更新 Conductor callback/lease。旧 attempt、过期 lease、取消 Job 的结果分别返回 409/410，不覆盖当前状态。

Completion/Failure 验证 Principal 绑定、attempt、lease、输出 schema 与 Object Path 后，事务写 Inbox 和 `result_received`，立即返回 `202 + receipt_id`。后台 Adapter 重试 `POST /api/tasks`；重复请求返回原 receipt，不要求客户端因 Conductor 暂时失败重新执行 Provider。

### 对象传输

Definition 与持久任务只保存 Object Path。领取时生成短时 presigned GET/PUT URL；Worker 上传后回传 Object Path、checksum、content_type、size 和 provenance，不能提交任意外部 URL。Provider URL 必须先摄取到受控存储。Finalize 校验对象后，以 `(step_run_id, attempt)` 幂等登记唯一 Generated Asset。

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

- Semantic Compiler 在 IR 中冻结 `candidate_count` 与 `approval_required`；Template 多候选/`needs_confirm` 或 Step `approval_policy=always` 均要求审批。
- Backend Compiler 在 Capability task 后插入 Conductor `HUMAN` task。`media_worker` 只返回 candidates，不消费 HUMAN、render_template 或 finalize 内部任务。
- media_workflow 对账 HUMAN task input，投影 `StepRun=need_confirm`；用户确认后以 `selected_candidate` 完成 HUMAN task，Finalize 形成唯一输出并登记 Generated Asset。
- 用户选择重新生成时，从当前 Step 的 Capability task rerun，清空本次候选并返回 running；不把重新生成误当失败重试。
