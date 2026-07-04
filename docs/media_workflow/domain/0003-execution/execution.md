---
name: execution
description: Job（执行任务）与 StepRun（步骤运行）持久化、执行编排
dependencies:
    - workflow
    - template
    - scope: media_auth 跨域术语
related_workflow:
    - workflow_execution
---

## 一句话职责
execution 域负责把一个已校验的 Workflow 实例化——落地为 Job + N 个 StepRun，调度 Template.Render，汇总 step 输出作为后续 step 输入，落地最终产物；维护 Job 状态机（pending / running / succeeded / failed / cancelled）。

### 不做什么
- 不管理 Workflow / Template 的版本（由对应 domain 负责）。
- 不持久化素材本身（产物内容存于 `media_materials` Asset；本域仅持 asset_id 引用）。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| SubmitJob | 提交一个 workflow_id + workflow_inputs；创建 Job 与初始 StepRun，等待调度 | `POST /api/v1/jobs` |
| GetJob | 读取 Job + 全部 StepRun | `GET /api/v1/jobs/{id}` |
| ListJobs | 按 scope/workflow_id/status 过滤分页 | `GET /api/v1/jobs?scope=&workflow_id=&status=` |
| CancelJob | 把 running/pending 改为 cancelled；已 succeeded/failed 不可 cancel | `POST /api/v1/jobs/{id}/cancel` |
| RetryJob | 把 failed Job 重置为 running，重跑失败 / skipped step；verifier+ on workflow | `POST /api/v1/jobs/{id}/retry` |
| RetryStepRun | 重跑单个 StepRun 与其下游；viewer+ on workflow | `POST /api/v1/jobs/{id}/steps/{step_run_id}/retry` |
| GetStepRun | 读单条 StepRun 状态与产物 | `GET /api/v1/jobs/{id}/steps/{step_run_id}` |

## 数据模型

### Tables — `jobs`

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| id | 主键 | bigserial | — |
| scope | 业务范围（沿用 workflow.scope） | text not null | 不可改 |
| workflow_id | FK | bigint not null | — |
| workflow_version | 不可变版本号 | int not null | 拒绝修改 workflow 后旧 Job 失效 |
| inputs | Job 入参快照 | jsonb not null | keys=WorkflowInput.name；values 按 workflow_input.`type` 取：`{material_id}` / `{asset_id}`(=generated_asset) / `{product_id}` / `{virtual_ip_id}`（const 则直接字面值） |
| status | 状态机 | text not null | pending / running / succeeded / failed / cancelled |
| retry_count | 已重试次数 | int not null | default 0；详见「Retry」一节 |
| retry_history | 重试历史 | jsonb not null | default '[]'；按 `[retry_at, from_status, reason, retry_inputs?]` 累积 |
| submitted_by | 提交者 | text not null | — |
| submitted_at | 提交时间 | timestamptz not null | — |
| started_at | 首次进入 running 的时间戳 | timestamptz nullable | — |
| finished_at | 终态时间戳 | timestamptz nullable | — |
| failure_reason | 失败原因 | text nullable | 仅 status=failed |

约束：INDEX `(scope, status, submitted_at desc)`；INDEX `(workflow_id, workflow_version)`；INDEX `(submitted_by, submitted_at desc)`。

### Tables — `step_runs`

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| id | 主键 | bigserial | — |
| job_id | FK | bigint not null | ON DELETE CASCADE |
| step_key | 节点名 | text not null | — |
| step_order | 顺序 | int not null | — |
| template_id | 引用的 Template | bigint not null | — |
| template_version | 引用的 Template 版本 | int not null | — |
| status | 状态机 | text not null | pending / running / succeeded / failed / skipped |
| retry_count | 已重试次数 | int not null | default 0 |
| resolved_inputs | 实际入参快照（已解析为 material/generated_asset） | jsonb not null | keys=Template.Inputs[].name；values=`{resource_type, id, asset_meta?}` 或 const。product / virtual_ip 输入在解析阶段已被还原为具体 material/generated_asset（按 slot.kind 选择） |
| rendered_prompt | 渲染后的 prompt 字符串 | text nullable | — |
| output_asset_id | 产出 generated_asset id | bigint nullable | 单一产出（Template 单 output_asset_kind）；指向 media_materials.generated_assets.id；upsert 时回填本 step_run 的 job_id/step_run_id/template_id/template_version 作为生成记录 |
| failure_reason | 失败原因 | text nullable | — |
| started_at / finished_at | 时间戳 | timestamptz nullable | — |

约束：UNIQUE `(job_id, step_key)`；INDEX `(job_id, status)`；INDEX `(template_id, template_version)`。

## 实体与状态机

```go
type JobStatus string
const (
    JobStatusPending   JobStatus = "pending"
    JobStatusRunning   JobStatus = "running"
    JobStatusSucceeded JobStatus = "succeeded"
    JobStatusFailed    JobStatus = "failed"
    JobStatusCancelled JobStatus = "cancelled"
)

type StepRunStatus string
const (
    StepRunStatusPending   StepRunStatus = "pending"
    StepRunStatusRunning   StepRunStatus = "running"
    StepRunStatusSucceeded StepRunStatus = "succeeded"
    StepRunStatusFailed    StepRunStatus = "failed"
    StepRunStatusSkipped   StepRunStatus = "skipped"
)
```

合法转移：

- Job：`pending → running → (succeeded | failed | cancelled)`；`failed → running`（retry 触发）；其余不可逆。
- StepRun：`pending → running → (succeeded | failed | skipped)`；`failed → running`（retry 触发）；其余不可逆。

> 与旧版（v1 之前）的区别：v1 旧版 `failed` 是终态不可再变，要重试必须新建 Job。本版在保留"提交新 Job"路径的同时，允许 in-place retry，详见下节。

## 行为约束

1. SubmitJob 时把 workflow 全部 Binding 重新校验一遍（含 scope / 输入可达性：asset 走 AssetReachability、product / virtual_ip 走各自 CheckAccessible），无错才能进入 pending。
2. 调度时按 step_order 升序逐个 Step 触发；每个 Step 完成后才能调度依赖它输出的下一 Step（用 binding 上的 `source=step_output` 反查）。
3. 同一 Step 内 Render 一次；Render 失败 → status=failed。
4. Job 的最终资源产物由执行流的 sink step（`OutputStepKey`）的 `output_asset_id` 给出；Job 不独立记录 output。
5. **Retry**：仅允许在 `status=failed` 状态下重试；不允许对 `succeeded` / `cancelled` Job 重试。

## Retry

`/api/v1/jobs/{id}/retry` 仅对 `status=failed` 开放（其余状态 409）；重新进入 `running` 状态。

### 行为

对 Job 调 `RetryJob(jobID, reason)`：
1. 校验 `Job.Status == failed`；否则拒绝。
2. 校验 `Job.RetryCount < MaxRetries`（默认 3，硬上限可在 service 配置里调）；超出拒绝 `ErrRetryLimitExceeded`。
3. 把 Job.status 回 `running`；`retry_count + 1`；`started_at = now()`；`finished_at = NULL`；`failure_reason = NULL`。
4. 把本次 retry 的快照 append 到 `retry_history`：`{retry_at: now(), reason, retry_inputs?: <user input overrides>}`。
5. 重新调度：
   - 所有 `step_run.status in {succeeded}` 保留（不再重跑）；
   - 所有 `step_run.status in {failed, skipped}` 重置为 `pending` 后重新入队——但 `retry_inputs` 可选覆盖该 step 的 `resolved_inputs`（典型：换 asset_id 重跑某一步）；
   - 依赖图（前驱 / 后继）仍按 `binding.source=step_output` 推导，但因为 succeeded step 没重置，下游不必重新等前驱。
6. step_run `retry_count + 1`。

如果调用方**只想重跑某个 Step**（不重置整 Job），使用 `POST /api/v1/jobs/{id}/steps/{step_run_id}/retry`：仅该 StepRun 重置 + 重新调度，下游 StepRun 也按需重置（当下游 `binding.source=step_output` 引用了本 step 时）。

### 权限

Retry 操作需要 `my_permission(user, workflow) ≥ viewer`（与 SubmitJob 一致）。同时，AssetReachability 重新走一遍——重试时的 asset 必须仍 Reachable；不可达则前置拒绝。

### 产物覆盖语义

Retry 时同一 StepRun 行的 `output_asset_id` 会被新 attempt 的产物覆盖；旧 attempt 产物存放在 `retry_history` 与 audit log（不在本 spec 范围）。

### 取消语义

Retry 不会**取消**已 succeeded 的历史 step——那些 StepRun 保留 succeeded、不重跑；retry 只重置 failed / skipped 的 StepRun。已 succeeded step 的 `output_asset_id` 在 retry 期间不被清空，仅当该 step 自身被重跑时才被新 attempt 产物覆盖（见上节「产物覆盖语义」）。

> 与"每次 retry 创建一个新 StepRun 行"的对比：本版用同一行 + retry_count 计数，避免历史 StepRun 表无限增长；如需保留历史 attempt 链，用 `retry_history` 数组读。

## 关联关系

## 关联关系

### 被依赖的 domain
无（execution 是叶子域）。

### 参与的 workflow
- [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md) — 这是 execution 域的主 workflow
- [workflow_validation](../../workflow/0001-workflow_validation/workflow_validation.md) — 间接：在 SubmitJob 时被调用

### 关联术语
[Execution](../../CONTEXT.md)、[Job](../../CONTEXT.md)、[Step Run](../../CONTEXT.md)。
