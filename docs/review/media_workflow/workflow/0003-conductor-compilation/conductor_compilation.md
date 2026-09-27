# Conductor Compilation 实现一致性审查

审查基线：`docs/ddd/media_workflow/workflow/0003-conductor-compilation/conductor_compilation.md`、ADR 0013、`docs/spec/enterprise-media-production.md`。审查对象：`biz/workflow/compiler` 与 Workflow Publish。

## 结论

纯函数 Compiler 已完成拓扑波次、动态并行/JOIN、单节点 SUB_WORKFLOW、稳定 Definition 名称与 hash 的基础算法，单元测试通过；但“可靠编译并注册”这个 Application Workflow 尚未实现。当前 Publish 只在内存中调用 Compile，随后直接把 Workflow 标为 published。

## 已实现

- 从 `STEP_OUTPUT` binding 构边并用 Kahn 算法形成拓扑波次。
- 多节点波次生成 `FORK_JOIN_DYNAMIC + JOIN`，单节点波次生成 `SUB_WORKFLOW`。
- 最后一波必须只有 output step。
- Definition 不包含具体 Job/StepRun ID。
- canonical JSON 与 SHA-256 definition hash 函数存在，且稳定性有测试。
- 本地 Conductor 源码示例也使用动态 fork 后 `JOIN joinOn: []`，该 JSON 形状本身没有发现冲突。

## 缺失与不一致

| 严重度 | 设计要求 | 当前实现 |
|:---|:---|:---|
| P0 | `draft -> publishing -> published/publish_failed` | 领域状态只有 draft/published/archived；Compile 成功立即 published |
| P0 | 事务内冻结快照、写 Compilation 与 Outbox | 没有 compilation/outbox 表、聚合或仓储 |
| P0 | 向 Conductor 注册并按远端 hash 处理冲突 | 没有 Conductor client/adapter；`DefinitionHash` 没有生产调用点 |
| P0 | 只有 registered 版本可 SubmitJob | SubmitJob 不读取 Compilation |
| P0 | 固定的 `media_step_execution_v1` 子工作流已注册 | 只有字符串常量，没有子工作流 Definition、DO_WHILE、WAIT 或注册代码 |
| P0 | SubmitJob 生成 `wave_n_step_inputs` 并启动 Definition | 全仓只有 Compiler/Test 出现该字段，运行路径从未生成 |
| P1 | task reference 遵循 `step__<sanitized_key>` | 单节点使用 `wave_n__<key>`；动态分支由输入列表隐式承载，映射审计信息不足 |
| P1 | 编译输入冻结 Template/Capability 版本和执行策略 | Compiler 只使用 DAG 依赖；Template、Capability、approval/reuse 没有进入 Definition 编译校验 |

## 判断

这部分目前属于 **compiler spike（编译算法验证）**，不能对外称为 Conductor 集成完成。合理的下一步不是扩展 Compiler JSON 字段，而是先实现 Compilation 聚合、状态机、Outbox 与 Conductor Adapter，然后用真实 Conductor 做注册/冲突/启动/恢复集成测试。
