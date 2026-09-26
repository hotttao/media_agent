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

新增独立 `media_worker` 可执行程序。它接受 Conductor URL、worker ID、启用的 Capability Task 列表和 profile；逐个长轮询 Capability Task，调用对应 Handler，并通过 `POST /api/tasks` 回报 `COMPLETED | FAILED`。生产 profile 调本地 Provider/CLI，`media_ai_fixture` profile 按 task type 返回冻结样本并附带 provenance。它不得轮询 `media_render_template_v1` 或 `media_finalize_step_v1`。

`media_workflow` 提供独立 internal-worker 入口执行 Renderer/finalizer。API 进程负责发布、注册与启动 Workflow，不在 HTTP goroutine 中执行 task polling。

## 持久化与版本

Template 与 Template Version 增加 `capability_key`、`capability_version` 和 `invocation_mapping jsonb`。新增版本化 `execution_capabilities`。`workflow_compilations` 保存源版本、Conductor name/version、definition_hash、状态与错误；运行时不从该表加载 IR。

## 测试

- Semantic Compiler：required 参数缺失、未知 Slot、类型错误、顺序稳定、上游 Step 引用、确定性 JSON。
- Renderer：动态 context、嵌套字段、缺失字段、非法模板、确定性 digest。
- Backend Compiler：Renderer 自动插入、Capability 参数引用正确、images 顺序、拓扑波次、hash 稳定、无敏感字段。
- 集成：注册 Definition 后，Renderer Worker 输出 prompt，Capability Worker 收到固定 `prompt + images[]`。
- E2E：迁移 `media_ai/scripts/workflow-cli` 的七阶段视频流程；只导入一个 fixture execution，启动真实 Conductor、internal worker 与 `media_worker --profile media_ai_fixture`，从 UI/API SubmitJob 并验证最终视频结果。用户确认前不迁移其余旧数据。

## Job 输入表单契约

- `InputSchemaEntry` 返回 `name/resource_type/asset_kind/required`；`asset_kind` 对 MATERIAL、ASSET 必填，对 PRODUCT、VIRTUAL_IP 可空。
- Workflow Validator 拒绝同名 MATERIAL/ASSET input 绑定不同 asset kind。
- 独立前端读取已发布 Workflow 的 scope、version 与 Input Schema，按 Organization/Scope/Asset Kind 查询候选并组装 JobInput；不向业务用户暴露 JSON 编辑器。
- MATERIAL 使用 object path 的可访问 URL 与文本内容形成 bundle；Product、Virtual IP 使用稳定业务字段形成快照。提交显式携带当前 Workflow version。
