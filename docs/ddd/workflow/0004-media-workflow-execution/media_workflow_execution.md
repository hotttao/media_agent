---
name: Media Workflow Execution
desc: 媒体工作流跨网关、客户端能力执行、Conductor、素材和存储服务的执行流程
---

## 一句话说明

`media_workflow` 持有 Job/StepRun 业务状态并在内网代理 Conductor；Capability Version 决定由经统一网关的 Personal Worker 或内部 Managed Worker 执行，产物先写对象存储，再由服务端 Finalizer 登记 Generated Asset。

## 执行时间线

```mermaid
sequenceDiagram
    participant B as Browser / Caller
    participant PW as Personal media_worker
    participant IW as Managed Worker
    participant G as Gateway + Edge Auth
    participant W as media_workflow
    participant DB as Workflow DB
    participant C as Conductor
    participant S as Storage
    participant M as Materials
    B->>G: SubmitJob(browser session)
    G->>W: Internal JWT
    W->>DB: TX1 Job + StepRuns + Start Outbox
    W->>C: StartWorkflow(taskToDomain by execution_target)
    alt personal_worker
        PW->>G: Poll(capabilities, API Token)
        G->>G: verify via media_auth/Talos + delegated JWT
        G->>W: Personal Worker poll
        W->>C: poll(taskDef, user domain)
        W->>DB: TX2 Personal WorkerDispatch
        W-->>PW: dispatch_id + sanitized input + presigned URLs
        PW->>S: PUT generated object
        PW->>G: Complete(dispatch_id, attempt, object metadata)
        G->>W: authenticated result
    else managed_worker
        IW->>W: internal poll(configured capability/pool)
        W->>C: poll(taskDef, managed domain)
        W->>DB: TX2 Managed WorkerDispatch
        W-->>IW: dispatch_id + sanitized input + presigned URLs
        IW->>S: PUT generated object
        IW->>W: internal Complete(dispatch_id, attempt, metadata)
    end
    W->>DB: TX3 Result Inbox + Dispatch=result_received
    W-->>MW: 202 receipt
    W->>C: async idempotent task update
    C-->>W: internal finalize task
    W->>S: verify object metadata
    W->>M: UpsertGeneratedAsset(step_run_id, attempt)
    W->>DB: TX4 StepRun terminal + unlock successors
```

TX2 必须发生在向客户端暴露任务之前；响应丢失时返回既有有效 Dispatch。TX3 先于 Conductor update，重复完成返回同一 receipt，Conductor 不可用时由 Adapter 重试而不是要求 Worker 重新执行 Provider。对象与 Asset 的事实分别由 Storage 和 Materials 拥有，Workflow 只保存稳定引用。

## 边界与失败

- Conductor、Storage 和 Materials 调用均不置于 Workflow 数据库事务中；本地状态、Inbox/Outbox 与业务事实按明确事务提交。
- Worker 上传完成但 completion 丢失时，未登记对象由 Storage retention/GC 处理；不得由客户端直接创建 Generated Asset。
- 取消阻止续租和新派发；运行中的本地 Provider 是否立即停止取决于 Worker 能力，晚到结果不得反转 cancelled Job。
- Conductor poll 后若服务在 Dispatch 持久化前崩溃，依靠 response timeout 重投；禁止无域 fallback 或从共享队列 poll 后再检查 owner。

## 状态、幂等与权限

Job 由 `pending/running` 进入 `succeeded/failed/cancelled`；StepRun 与 WorkerDispatch 的状态转换归 media_workflow Execution Domain。Job 用 `(workspace_id, idempotency_key)` 去重，Worker 结果用 `(dispatch_id, attempt, operation)` 去重，Asset 登记用 `(step_run_id, attempt)` 去重。Personal 路径由 Gateway 验证 API Token，media_workflow 再校验 User、Worker Actor、scope、Organization、Job 和 Personal Capability；Managed 路径校验内部 Worker 配置、Pool 和 Managed Capability。Materials 对 Generated Asset 登记执行最终资源授权。
