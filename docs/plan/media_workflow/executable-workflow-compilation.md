# Executable Workflow 编译落地计划

## P1 · 领域类型与纯函数

- [x] 增加 Invocation Mapping 表达式与校验。
- [x] 增加 `ExecutableWorkflowIR` / `ExecutableStepIR`。
- [x] 实现确定性 Template Renderer 与 digest。
- [x] 单元测试覆盖动态 context、映射顺序和错误边界。

## P2 · Semantic Compiler

- [x] 扩展 Template/Capability 冻结快照。
- [x] 支持显式 Workflow Input 引用，并将 API 统一为 `slot_bindings`。
- [x] 编译 Slot Binding、Context 与 Invocation Mapping。
- [x] required 覆盖和类型检查；生成确定性 IR 结构。

## P3 · Conductor Backend Compiler

- [x] 以 IR 替换当前直接消费 Workflow 的 compiler 入口。
- [x] 自动插入 `media_render_template_v1`。
- [x] 生成具体 Capability Task 与有序参数引用。
- [x] 保留拓扑波次、唯一输出和稳定 Definition Hash。

## P4 · 持久化与发布

- [x] migration：Capability、Template Capability 引用、Invocation Mapping、Compilation。
- [ ] Publication Outbox 与失败注册后台重试。
- [x] 发布状态机与快照冻结。
- [ ] Conductor 注册冲突 hash 比对和自动重试。
- [x] 只有 registered compilation 可 SubmitJob。

## P5 · Renderer Worker 与联调

- [x] 独立 Worker 进程轮询 `media_render_template_v1`。
- [x] 接入纯 Renderer，完成 task complete/fail 与确定性 digest。
- [ ] internal worker 的 trace_id 透传和任务指标。
- [x] 注册内部 Task Definition。
- [x] 使用真实 Conductor 验证 `业务 Slot -> prompt/images[] -> Capability`。

## P6 · 客户端 Media Worker 与旧流程样本

- [x] 新建客户端 `media_worker`，实现 Conductor poll/update、worker_id、幂等与优雅退出。
- [x] 实现 `media_ai_fixture` Mock Profile，冻结一个已完成视频流程的结果与 provenance。
- [x] 迁移七阶段 `model-image -> style-image -> first-frame -> video -> clip -> ai-fill -> publish` Workflow、Templates 和 Capability 引用。
- [x] 只通过 API 导入并执行一个完整样本；验证 Job、Conductor tasks 与最终视频结果。
- [ ] 等用户确认后再迁移全部旧数据。

## P8 · 人工审批闭环

- [x] IR v2 冻结候选数量与是否需要审批。
- [x] Backend Compiler 在需要审批的 Capability 后插入 HUMAN task。
- [x] 对账 HUMAN input 为 StepRun candidates/need_confirm，并从 UI 确认唯一候选。
- [x] 确认后完成 HUMAN task，Finalize 登记 Generated Asset 并继续 DAG。
- [x] 重新生成从当前 Capability task rerun；Cancel/Retry 同步控制 Conductor execution。

## P7 · 可直接使用的 Job 输入表单

- [x] Input Schema 增加 asset_kind，并校验同名 material/asset input 的类别一致性。
- [x] 回填既有 Workflow 的 Input Schema asset_kind。
- [x] 前端按 Workflow Input Schema 查询 Product、Virtual IP、Material 与 Generated Asset 候选。
- [x] 用资源选择表单组装 JobInput，移除业务用户可见的 JSON 编辑器。
- [x] 覆盖加载、空态、失败、必填校验、Workflow 切换和窄屏布局。
- [x] 通过组件/单元测试和实际账号 API 联调验证，不执行第二条旧 Workflow 样本。
