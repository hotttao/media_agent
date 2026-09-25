# 使用显式 Executable Workflow IR 隔离业务语义与 Conductor

Workflow 的业务节点使用 Template Slot 和 Step Binding 表达输入输出依赖，而 Capability Worker 使用 `prompt:string`、`images:ImageRef[]` 等固定技术契约。采用两阶段编译：Semantic Compiler 把冻结的 Workflow、Template、Invocation Mapping 和 Execution Capability 转换为显式 `ExecutableWorkflowIR`；Conductor Backend Compiler 再把 IR 降低为 Conductor Workflow Definition。

`ExecutableWorkflowIR` 包含多个 `ExecutableStepIR`。每个 Step IR 固定依赖、Template 快照、渲染 context binding、Capability 版本、Capability 参数表达式、唯一输出类别及审批/复用策略。禁止使用 `CompiledStepPlan` 作为并列概念。IR 是不可变编译产物，不是业务聚合、编辑源或运行状态事实源。

Template Version 拥有 Invocation Mapping。例如 `prompt <- render_template()`，`images <- [asset_ref(person), asset_ref(garment)]`。Semantic Compiler 在发布期验证 Mapping 覆盖 Capability 必填参数且结果类型兼容。实际资源只在运行期进入表达式求值，Provider secret、临时 URL 和用户凭证不进入 IR 或 Conductor Definition。

Conductor Definition 内嵌各 `ExecutableStepIR` 降低后的执行语义。模板渲染使用编译器自动插入的内部任务 `media_render_template_v1`，其稳定契约为 `template + context map -> prompt`；动态 Slot 名只是 context 的键，不改变任务契约。图片数组等 Capability 参数由 Backend Compiler 按 Invocation Mapping 直接生成，Renderer 不猜测参数。`workflow_compilations` 只保存发布映射、状态与 Definition Hash，运行时不从该表加载转换计划。

该方案使领域模型不依赖 FlowGram 或 Conductor，发布期能够完整做类型检查，同一 Workflow 版本可确定性重放。代价是 Definition 体积增加，并需要维护 IR schema/version 与降低器；若未来 Definition 体积达到实际限制，再以保持相同 IR 语义的引用载体替换内嵌方式。

**状态：accepted**
