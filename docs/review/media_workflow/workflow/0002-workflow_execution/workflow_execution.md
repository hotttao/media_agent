# Workflow Execution 实现一致性审查

审查基线：`docs/ddd/media_workflow/workflow/0002-workflow_execution/workflow_execution.md`、`docs/spec/enterprise-media-production.md`。审查对象：Job/StepRun domain、service、handler、migration 与仓储。

## 结论

当前生产路径仍是旧的 `StepRun -> HTTP claim -> Worker complete/fail` 执行器，不是设计中的 `Job/Outbox -> Conductor -> Step Fragment -> Capability Worker`。基础的 DAG 推进和候选选择已存在，但缺少持久化 attempt、Decision、Outbox 与终态保护，不能满足重试、恢复和至少一次投递的一致性要求。

## P0 差异

1. **SubmitJob 绕过 Conductor。** Submit 只创建 Job/StepRun；不存在 StartWorkflow Outbox、Compilation 映射读取、correlationId 查询或 Conductor workflow id。Worker 继续调用公开的 claim/complete/fail API。
2. **未限制为已发布且已注册的 Workflow。** Submit 只比较当前版本号，没有检查 `workflow.status=published` 或 Compilation `registered`。
3. **缺少 Workspace 与请求幂等边界。** `jobs` 没有 `workspace_id`、`idempotency_key` 唯一约束；重复请求会创建新 Job。Handler 还从请求体读取 `submitted_by`，没有固定使用可信 principal。
4. **Candidate Review 不是审计聚合。** Candidate 以 JSON 覆盖保存在 StepRun；regenerate 清空候选，没有 attempt、feedback、Decision 表、幂等键或旧 attempt 晚到保护。
5. **终态可能被晚到完成反转。** Cancel 只把 Job 标为 cancelled，不冻结 StepRun；后续 Worker 完成仍会落 Generated Asset。并行分支失败后，另一个分支晚到成功时，`checkJobCompletion` 把 `succeeded + skipped` 汇总为 Job succeeded，可能覆盖 failed/cancelled。
6. **跨服务写入不是可靠流程。** `finalizeStepRun` 先同步调用 Materials，再分多次更新 StepRun/Job，错误被多处忽略；没有业务幂等键、Outbox/Inbox 或补偿。响应丢失可能重复创建资源，局部失败可能留下无法恢复的状态。
7. **执行权限未落地。** Job 查询、取消、重试、确认和 Worker API 的 service 方法不接收 caller/workspace；无法执行最终资源授权。Team grant 也未参与 Job 访问判断。

## P1 差异

- `reuse_policy` 已进入 Workflow Step 表，但执行路径从未读取或计算复用指纹，`reuse_if_available` 实际无效。
- 审批策略部分覆盖：多个 Candidate 会等待，`always` 对单 Candidate 生效；但旧的 Template `needs_confirm` 仍可强制审批，导致审批规则同时由 Template 与 Step 两处决定。
- regenerate 没有反馈参数，也不递增 attempt，更没有强制下一次 `force_new`。
- Worker 路由仍按自由字符串 `platform/gen_way/gen_type` 领取任务，未通过版本化 Execution Capability 和 Runtime Environment 授权。
- StepRun 状态仍是 `claimed/need_confirm` 旧模型，缺少 `dispatching/running/confirming/regenerating`，无法表达外部副作用的不确定窗口。
- 读取前驱结果依赖前驱 Candidate/selected_index，而不是只读取已经登记的唯一 `output_resource_id`。

## 能力覆盖矩阵

| 原需求能力 | 目标 Domain/Workflow | 状态 |
|:---|:---|:---|
| 创建 Job 与 StepRun | execution / workflow_execution | covered |
| 本地 DAG 推进 | execution | covered（旧实现） |
| 多 Candidate 强制确认 | execution | covered（无 attempt 审计） |
| 单 Candidate 可配置审批 | workflow Step policy | covered |
| 带反馈重新生成 | Step Review Decision | missing |
| 同一 StepRun 新 attempt | execution | missing |
| 结果复用 | generation reuse workflow | missing |
| Conductor 启动与恢复 | workflow_execution | missing |
| Worker 服务身份与 Workspace/Capability 授权 | Worker Gateway | missing |
| 至少一次结果幂等 | execution + Inbox/Outbox | missing |
| cancel/failed 终态不被晚到事件反转 | execution aggregate | missing |

## 建议

不要继续给旧 ClaimStepRun 路径叠加 Conductor 逻辑。应建立新 Execution 聚合表和 Outbox/Inbox，让旧 Job 仅负责历史收尾；新 Workspace 通过 feature flag 进入 Conductor 路径。第一批实现必须先解决终态 CAS、attempt/decision 幂等和 Generated Asset 幂等，再接 WAIT UI。
