---
name: generation
description: 图片、音频和视频生成任务及候选产物
dependencies:
    - media_storage/stored_object
related_workflow:
    - job-execution-loop
    - media_workflow_execution
---

## 一句话职责

Generation 负责一次媒体生成任务的持久化、执行、重试和候选对象路径，是生成状态的唯一事实源。

## 边界

### 做什么
- 按幂等键创建 Generation Task。
- 调用具体 Provider，写候选对象到 Storage。
- 管理取消、失败重试、多候选确认、带反馈重新生成和结果事件。

### 不做什么
- 不拥有 Workflow StepRun 或 Material 记录。
- 不保存完整存储 URL，不把 River Job 当业务事实。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建生成任务 | 保存输入快照与路由 | `GenerationService.CreateTask` |
| 查询生成任务 | 返回状态和候选路径 | `GetTask/ListTasks` |
| 取消任务 | 取消未完成执行 | `CancelTask` |
| 确认候选 | 选定一个候选结果 | `ConfirmCandidate` |
| 重新生成 | 保留旧候选，记录反馈并开始新的生成 attempt | 待 IDL 定义 |
| 执行任务 | Worker 调 Provider 与 Storage | `ExecuteGenerationTask` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Generation Task | Candidate、Generation Review | 单次执行结果、确认或重新生成决定 | 一个 Task 可以产生多个 Candidate，但最多一个 Primary Candidate；只有 Primary Candidate 构成最终单一输出 |

- 多候选执行完成后进入 `needs_confirm`，不会自动产生多个 Workflow Step 输出。
- `accept(candidate)` 只能选择当前 attempt 的 available Candidate，并原子设置 `selected_candidate_id` 与 Task succeeded。
- `regenerate(feedback)` 只允许从 `needs_confirm` 发起；保留旧 Candidate，递增 attempt 并回到 pending。
- 创意重新生成与失败重试不同：重新生成是用户决定，必须创建新的 attempt；失败重试可以继续同一业务请求的恢复策略。
- 重复确认同一 Candidate 幂等成功；Task 已选择其他 Candidate 后再确认返回冲突。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| GenerationCandidatesReady | 多候选已持久化，Task 进入 needs_confirm | task_id, attempt, candidate_ids | media_workflow |
| GenerationCandidateConfirmed | Primary Candidate 已确定 | task_id, attempt, candidate_id, object_path | media_workflow、media_materials |
| GenerationRegenerationAccepted | 重新生成决定已接受并创建新 attempt | task_id, previous_attempt, new_attempt | media_workflow |
| GenerationTaskFailed | Task 执行失败 | task_id, attempt, error_code | media_workflow |

## 数据模型

### `generation_tasks`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Generation Task ID |
| workspace_id | text | 所属 Workspace |
| requested_by | text | 发起用户 ID |
| source | text | workflow/manual 等来源 |
| source_resource_id | text | 来源 Step Run 或业务记录 ID |
| idempotency_key | text | 来源内幂等键 |
| media_type | text | image/audio/video |
| provider_id | text | 生成 Provider ID |
| input | jsonb | Prompt、输入 Object Path 和生成参数 |
| status | text | pending/running/needs_confirm/succeeded/failed/cancelled |
| attempt | integer | 当前执行次数 |
| selected_candidate_id | text nullable | 最终候选 ID |
| latest_feedback | text nullable | 最近一次重新生成反馈；完整历史见 generation_reviews |
| error_code | text nullable | 稳定错误码 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

### `generation_candidates`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Candidate ID |
| task_id | text | 所属 Generation Task |
| attempt | integer | 产生该 Candidate 的生成 attempt |
| object_path | text | Storage 中的结果路径 |
| candidate_index | integer | Task 内候选序号 |
| metadata | jsonb | 尺寸、时长、Provider 信息等 |
| status | text | available/selected/superseded/deleted |
| created_at | timestamptz | 创建时间 |

### `generation_reviews`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Review ID |
| task_id | text | 所属 Generation Task |
| attempt | integer | 被确认的 attempt |
| decision | text | accept/regenerate |
| candidate_id | text nullable | accept 时选择的 Candidate |
| feedback | text nullable | regenerate 时传给下一 attempt 的用户反馈 |
| decided_by | text | 作出决定的 User ID |
| idempotency_key | text | 确认命令幂等键 |
| created_at | timestamptz | 决定时间 |

状态：`pending -> running -> needs_confirm | succeeded | failed | cancelled`，并允许 `needs_confirm -> pending` 进入新的生成 attempt。`(source, source_resource_id, idempotency_key)` 与 `(task_id, idempotency_key)` 分别约束任务创建和 Review 命令幂等。

### UUID 生成规则

Task 与 Candidate 使用 ULID；Storage 路径由 `generation/{workspace_id}/{task_id}/{candidate_id}` 派生。

## 能力详细说明

### ExecuteGenerationTask

1. 原子领取 pending Task，记录 attempt；事务外调用 Provider。
2. 每个候选流式写 Storage，数据库只保存 object_path。
3. 单候选直接 succeeded；多候选进入 needs_confirm。
4. 本地状态与完成/失败 Outbox 同事务，重复执行按 Task/attempt 幂等。

### ConfirmCandidate

1. 校验 Task 为 `needs_confirm`，Candidate 属于当前 Task 与当前 attempt 且状态为 available。
2. 同一事务写 Generation Review、设置唯一 `selected_candidate_id`、把所选 Candidate 标为 selected、其余当前 attempt Candidate 标为 superseded，并提交 succeeded 与 Outbox。
3. 重复确认同一 Candidate 按幂等键返回既有结果；不同 Candidate 与既有选择冲突。

### Regenerate

1. 校验 Task 为 `needs_confirm`，记录 `regenerate(feedback)` Review。
2. 当前 attempt 未选 Candidate 标为 superseded；Task `attempt + 1`、状态回到 pending，清空终态错误但保留全部历史 Candidate。
3. 新 attempt 使用原始输入和用户 feedback；即使输入指纹相同也不得命中上一 attempt 的复用结果。
