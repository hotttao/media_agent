---
name: transcription
description: 音视频对象的异步语音转写任务
dependencies:
    - media_storage/stored_object
related_workflow: []
---

## 一句话职责

Transcription 负责把 Workspace 内音视频对象转为文本并持久化任务状态和结果。

## 边界

### 做什么
- 创建、查询、取消和重试 Transcription Task。
- 从 Storage 读取输入对象并调用 ASR Provider。
- 保存文本、时间片段和完成事件。

### 不做什么
- 不拥有输入媒体的业务 ACL。
- 不把转写任务混入 Generation Task。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建转写任务 | 保存输入对象路径与语言参数 | `TranscriptionService.CreateTask` |
| 查询转写 | 返回状态和文本结果 | `GetTask/ListTasks` |
| 取消转写 | 取消未完成任务 | `CancelTask` |
| 执行转写 | Worker 调用 ASR Provider | `ExecuteTranscriptionTask` |

## 数据模型

### `transcription_tasks`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Transcription Task ID |
| workspace_id | text | 所属 Workspace |
| requested_by | text | 发起用户 ID |
| input_object_path | text | 输入音视频的 Storage 路径 |
| language | text nullable | 指定语言；空表示自动识别 |
| provider_id | text | ASR Provider ID |
| idempotency_key | text | Workspace 内幂等键 |
| status | text | pending/running/succeeded/failed/cancelled |
| transcript | text nullable | 较短的完整转写文本 |
| transcript_object_path | text nullable | 较大结果的 Storage 路径 |
| segments | jsonb nullable | 带时间范围的文本片段 |
| attempt | integer | 当前执行次数 |
| error_code | text nullable | 稳定错误码 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

### UUID 生成规则

Task 使用 ULID；较大 transcript 可写 Storage 并以 object_path 引用。

## 能力详细说明

### ExecuteTranscriptionTask

验证 Task 仍可运行，事务外从 Storage 读取对象并调用 ASR；随后事务内保存结果与 Outbox。Provider 超时按 attempt 重试，非法媒体类型直接失败。
