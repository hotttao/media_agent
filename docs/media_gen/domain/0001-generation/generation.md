---
name: generation
description: 单次 StepRun 生成执行域--作为 media_workflow 外部 worker，按 platform/gen_way claim StepRun、产候选、回写 Complete/Fail
dependencies:
    - media_workflow.execution
related_workflow:
    - job-execution-loop
---

## 一句话职责
generation 域负责单次 StepRun 的执行：调 `media_workflow.ClaimStepRun` 按 `(platform, gen_way, gen_type)` 匹配领取 -> 本地执行生成（本期 mock 产假候选 url/content）-> 调 `media_workflow.CompleteStepRun` 回写候选（`needs_confirm=true` 产 N 候选，否则产 1 个 `is_primary=true` 主候选）或 `FailStepRun` 回写失败原因。本身**不持久化**任何状态，job 状态机归 `media_workflow`。

## 边界

### 做什么
- 调 `media_workflow.ClaimStepRun` 按 `(platform, gen_way, gen_type, worker_id)` 领取 pending StepRun，拿到 `rendered_prompt` + `input_assets` + `candidate_count`。
- 执行生成（本期 mock：按 `candidate_count` 产 N 个假候选 url/content，url 形如 `mock://generated/{step_run_id}/{index}`）。
- 按 `candidate_count` 决定产出形态：
  - `candidate_count > 1`：产 N 候选，`is_primary=false`（交用户选）。
  - `candidate_count == 1`：产 1 候选，`is_primary=true`（直接主图）。
- 调 `media_workflow.CompleteStepRun` 回写候选，或 `FailStepRun` 回写失败原因（mock 不会失败，除非 Claim 返回空）。
- Claim 返回空（无待领 StepRun）时静默返回，不报错。

### 不做什么
- 不持久化候选产物、worker 状态、执行历史（全部状态归 `media_workflow.jobs` / `step_runs` 表）。
- 不实际调下游生成器（GPT / 即梦 / 其他）--本期 mock，留 stub 接口。
- 不落地产物到 `media_materials.generated_assets`--产物落地由 `media_workflow` 在 `CompleteStepRun` 内调 `UpsertGeneratedAsset` 完成（见 [workflow_execution](../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md)）。
- 不管理 StepRun 状态机--仅触发转移（claimed -> succeeded/failed），状态机归属 `media_workflow.execution`。
- 不渲染 prompt--`rendered_prompt` 由 `media_workflow` 在 Claim 时已渲染完成，本域直接消费。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| ExecuteOnce | 单次 Claim -> 生成(mock) -> Complete/Fail 循环；幂等可重复调用 | `service.ExecuteOnce(ctx, workerID)` |
| GenerateCandidates | 本地生成(mock) 候选：按 candidate_count 产 N 个假候选 | `service.GenerateCandidates(stepRunID, renderedPrompt, count)` |
| ClaimFromWorkflow | 调 `media_workflow.ClaimStepRun` RPC 领取 StepRun | `service.ClaimFromWorkflow(ctx, q)` |
| CompleteToWorkflow | 调 `media_workflow.CompleteStepRun` RPC 回写候选 | `service.CompleteToWorkflow(ctx, stepRunID, candidates)` |
| FailToWorkflow | 调 `media_workflow.FailStepRun` RPC 回写失败 | `service.FailToWorkflow(ctx, stepRunID, reason)` |

> 本域不直接暴露 HTTP RPC：worker loop 由 `worker.proto` 的 StartWorker/StopWorker/WorkerStatus 控制，loop 内部调用本域 service 方法。`media_workflow` 的 Claim/Complete/Fail RPC 由本域 service 通过 `hertz_gen/media_workflow/job_service` client 调用。

## 数据模型

**无表**。本域不持久化，全部状态归 `media_workflow` 的 `jobs` / `step_runs` 表（见 [media_workflow execution](../../media_workflow/domain/0003-execution/execution.md#tables---step_runs)）。

> 设计决策：media_gen 是无状态执行单元，避免与 `media_workflow` 维护的 job 状态机重复。优点：worker 可水平扩缩、进程崩溃不丢状态（Claim 超时由 `media_workflow` scanner 回收）；缺点：每次执行需 RPC 往返 `media_workflow`。回退成本（如改为本地持久化）不高，未达 ADR 阈值。

## 实体与值对象

```go
// ClaimQuery -- 调 media_workflow.ClaimStepRun 的入参（对齐 ClaimStepRunReq）
type ClaimQuery struct {
    WorkerID string
    Platform string
    GenWay   string
    GenType  string
}

// ClaimResult -- Claim 返回的 StepRun 执行上下文（对齐 ClaimStepRunResp）
type ClaimResult struct {
    StepRunID      string
    JobID          string
    RenderedPrompt string
    InputAssets    []InputAsset
    Platform       string
    GenWay         string
    GenType        string
    AssetKind      string
    CandidateCount int
}

type InputAsset struct {
    Label   string
    URL     string
    Content string
}

// Candidate -- 产物候选，对齐 media_workflow.job.Candidate
type Candidate struct {
    Index     int
    URL       string
    Content   string
    IsPrimary bool
}
```

## 能力详细说明

### ExecuteOnce
```
入参：workerID, platform, gen_way, gen_type
1. ClaimFromWorkflow(workerID, platform, gen_way, gen_type)
   - 调 media_workflow.ClaimStepRun RPC
   - 若返回 step_run_id == "" -> 无待领，return nil（非错误）
2. GenerateCandidates(stepRunID, renderedPrompt, candidateCount)
   - mock: 按 count 产 N 个 Candidate{
       index: i,
       url:   "mock://generated/{stepRunID}/{i}",
       content: "mock content for {stepRunID}#{i}",
       is_primary: count == 1   // count==1 -> 主候选；count>1 -> 全 false
     }
3. CompleteToWorkflow(stepRunID, candidates)
   - 调 media_workflow.CompleteStepRun RPC
   - 若失败 -> FailToWorkflow(stepRunID, "complete failed: " + err)
4. return nil
异常：
- Claim RPC 失败 -> 直接 return err（不 Fail，因为没拿到 stepRunID）
- Generate 失败 -> FailToWorkflow(stepRunID, reason)
- Complete RPC 失败 -> 不再 retry（media_workflow scanner 会回收 claimed 超时的 step_run）
```

### GenerateCandidates（mock）
```
入参：stepRunID, renderedPrompt, count
1. count <= 0 -> 默认 1
2. for i in [0, count):
     candidates[i] = Candidate{
       Index: i,
       URL: "mock://generated/{stepRunID}/{i}",
       Content: "mock content for {stepRunID}#{i}",
       IsPrimary: count == 1
     }
3. return candidates
注：本期 mock 不消费 renderedPrompt（真实实现会调下游生成器）。
```

## 跨服务 RPC 列表

| RPC 方向 | 目的 |
|:---|:---|
| `media_gen` -> `media_workflow` `ClaimStepRun` | worker loop 拉取待执行 StepRun |
| `media_gen` -> `media_workflow` `CompleteStepRun` | 回写候选产物 |
| `media_gen` -> `media_workflow` `FailStepRun` | 回写执行失败原因 |

> client stub 位置：`hertz_gen/media_workflow/job_service/`（由根 `make hz-client` 生成）。本服务通过 `biz/shared/client/` 装配实例，service 层不直接 import client stub。

## 关联关系

### 依赖的 domain
- 无本服务内 domain 依赖。

### 被依赖的 domain
- [publish](../0002-publish/publish.md) - 不直接依赖 generation，但发布动作的输入 asset 通常来自 generation 产物（由调用方在 BFF 层组装 asset_id）。

### 参与的 workflow
- [job-execution-loop](../../workflow/0001-job-execution-loop/job-execution-loop.md) - worker 轮询循环，调用本域 ExecuteOnce

### 关联术语
[Generation Worker](../../CONTEXT.md)、[Candidate](../../CONTEXT.md)、[Claim / Complete / Fail](../../CONTEXT.md)。
