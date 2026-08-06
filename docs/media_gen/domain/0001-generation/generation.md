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
- 管理取消、重试、候选确认和结果事件。

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
| 执行任务 | Worker 调 Provider 与 Storage | `ExecuteGenerationTask` |

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
| error_code | text nullable | 稳定错误码 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

### `generation_candidates`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Candidate ID |
| task_id | text | 所属 Generation Task |
| object_path | text | Storage 中的结果路径 |
| candidate_index | integer | Task 内候选序号 |
| metadata | jsonb | 尺寸、时长、Provider 信息等 |
| status | text | available/selected/rejected/deleted |
| created_at | timestamptz | 创建时间 |

状态：`pending -> running -> needs_confirm | succeeded | failed | cancelled`。`(source, source_resource_id, idempotency_key)` 唯一。

### UUID 生成规则

Task 与 Candidate 使用 ULID；Storage 路径由 `generation/{workspace_id}/{task_id}/{candidate_id}` 派生。

## 能力详细说明

### ExecuteGenerationTask

1. 原子领取 pending Task，记录 attempt；事务外调用 Provider。
2. 每个候选流式写 Storage，数据库只保存 object_path。
3. 单候选直接 succeeded；多候选进入 needs_confirm。
4. 本地状态与完成/失败 Outbox 同事务，重复执行按 Task/attempt 幂等。
