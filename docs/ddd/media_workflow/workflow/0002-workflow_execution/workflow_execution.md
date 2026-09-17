---
name: Workflow Execution
desc: media_workflow 内部把 Workflow DAG 推进为跨服务任务的流程
---

## 一句话说明

Execution 通过本地 Job/StepRun 状态机和 Outbox 向 Crawler/Gen 发出幂等命令，再按事件推进 DAG；不再由外部 Worker 领取 StepRun。

## 执行时间线

```mermaid
sequenceDiagram
    participant C as Caller
    participant W as media_workflow
    participant DB as Workflow DB
    participant D as Dispatcher
    participant X as Crawler / Gen
    C->>W: SubmitJob(identity, workflow_version, inputs, key)
    W->>W: Validate + Reachability result check
    W->>DB: TX1: Job + StepRuns + root Outbox
    D->>DB: claim command
    D->>X: CreateTask(step_run_id, attempt, inputs)
    X-->>D: downstream_task_id / existing / unknown
    D->>DB: TX2: StepRun=running + task_id
    X-->>W: Task succeeded / candidates ready / failed event
    alt 单一结果
        W->>DB: TX3: Inbox + StepRun result + successor Outbox/Job terminal
    else 多候选等待用户确认
        W->>DB: TX3: Inbox + StepRun=needs_confirm
        C->>W: accept(candidate, key) / regenerate(feedback, key)
        W->>DB: TX4: Step Review Decision + command Outbox
        D->>X: ConfirmCandidate / Regenerate
        X-->>W: candidate confirmed / regeneration accepted
        W->>DB: TX5: Inbox + unique output or next attempt
    end
```

下游结果不明确时按 `step_run_id + attempt` 查询，禁止直接创建第二个任务。Execution Domain 决定 Job/StepRun 转换；Dispatcher 和事件处理器只协调命令与结果。

## 状态流转

```mermaid
stateDiagram-v2
    [*] --> blocked
    blocked --> pending: predecessors succeeded
    pending --> dispatching
    dispatching --> running
    running --> needs_confirm
    running --> succeeded
    running --> failed
    needs_confirm --> confirming: accept(candidate)
    confirming --> succeeded: candidate confirmed + asset registered
    needs_confirm --> regenerating: regenerate(feedback)
    regenerating --> pending: new attempt accepted
    failed --> pending: retry
    blocked --> skipped: upstream terminal failure
```

## 幂等设计

- Job 使用 `(workspace_id, idempotency_key)` 去重。
- 下游创建命令使用 `step_run_id + attempt` 去重。
- 完成事件使用 event_id/下游 task ID 去重；旧 attempt 不覆盖新状态。
- 用户确认使用 `step_run_id + review idempotency_key` 去重；同一 attempt 的 accept 与 regenerate 互斥。
- 本地状态与 Outbox 同事务，外部 RPC 不在数据库事务中执行。

## 一致性、恢复与权限

TX1 创建完整初始 DAG 运行事实；TX2 绑定下游任务；TX3 处理普通结果或进入 needs_confirm；TX4 原子保存用户决定与命令 Outbox；TX5 应用下游确认结果并提交唯一输出或新 attempt。Outbox/Inbox 至少一次，旧 attempt 结果只审计。取消阻止未开始 StepRun，运行中下游按其取消能力处理，晚到成功不得反转 cancelled Job。用户/Workspace 在 Job 创建时固定；确认时重新校验当前用户的 Workspace 执行权限。Dispatcher 使用 mTLS 服务身份，下游根据受控代表上下文执行最终资源授权。
