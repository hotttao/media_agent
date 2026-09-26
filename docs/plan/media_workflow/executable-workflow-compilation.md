# Executable Workflow 编译落地计划

## P1 · 领域类型与纯函数

- [x] 增加 Invocation Mapping 表达式与校验。
- [x] 增加 `ExecutableWorkflowIR` / `ExecutableStepIR`。
- [x] 实现确定性 Template Renderer 与 digest。
- [x] 单元测试覆盖动态 context、映射顺序和错误边界。

## P2 · Semantic Compiler

- [ ] 扩展 Template/Capability 冻结快照。
- [x] 支持显式 Workflow Input 引用，并将 API 统一为 `slot_bindings`。
- [x] 编译 Slot Binding、Context 与 Invocation Mapping。
- [x] required 覆盖和类型检查；生成确定性 IR 结构。

## P3 · Conductor Backend Compiler

- [ ] 以 IR 替换当前直接消费 Workflow 的 compiler 入口。
- [x] 自动插入 `media_render_template_v1`。
- [x] 生成具体 Capability Task 与有序参数引用。
- [x] 保留拓扑波次、唯一输出和稳定 Definition Hash。

## P4 · 持久化与发布

- [ ] migration：Capability、Template Capability 引用、Invocation Mapping、Compilation/Outbox。
- [ ] 发布状态机与快照冻结。
- [ ] Conductor 注册、冲突 hash 比对和重试。
- [ ] 只有 registered compilation 可 SubmitJob。

## P5 · Renderer Worker 与联调

- [ ] 独立 Worker 进程轮询 `media_render_template_v1`。
- [ ] 接入纯 Renderer，完成 ack/fail、trace_id 和指标。
- [ ] 注册内部 Task Definition。
- [ ] 使用真实 Conductor 验证 `业务 Slot -> prompt/images[] -> Capability`。

## P6 · 客户端 Media Worker 与旧流程样本

- [ ] 新建客户端 `media_worker`，实现 Conductor poll/update、worker_id、幂等与优雅退出。
- [ ] 实现 `media_ai_fixture` Mock Profile，冻结一个已完成视频流程的结果与 provenance。
- [ ] 迁移七阶段 `model-image -> style-image -> first-frame -> video -> clip -> ai-fill -> publish` Workflow、Templates 和 Capability 引用。
- [ ] 只通过 API 导入并执行一个完整样本；验证 Job、Conductor tasks 与最终视频结果。
- [ ] 等用户确认后再迁移全部旧数据。
