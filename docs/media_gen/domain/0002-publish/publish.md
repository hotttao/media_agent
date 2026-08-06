---
name: publish
description: 视频发布任务、平台结果与生命周期
dependencies:
  - media_materials/asset
related_workflow:
  - video-publish
---

## 一句话职责

Publish 持有一次视频向目标平台发布的业务任务、稳定平台请求标识和最终发布结果。

## 边界

### 做什么
- 创建、查询和取消 Publish Task。
- 维护发布状态、attempt、平台发布 ID、URL 和错误分类。
- 根据稳定幂等键防止同一发布意图重复执行。

### 不做什么
- 不拥有视频 Asset 或二进制对象，归 `media_materials` 和 `media_storage`。
- 不自动从 Generation 触发发布，必须由用户或上层 Workflow 显式提交。
- 不把平台 API 调用放入数据库事务。
- 不委托 `media_crawler` 发布；目标平台 Adapter 是 media_gen 的基础设施实现。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建发布任务 | 校验视频引用并持久化发布意图 | 待 IDL 定义 |
| 查询发布任务 | 返回状态和平台结果 | 待 IDL 定义 |
| 取消发布任务 | 取消尚未进入不可逆发布阶段的任务 | 待 IDL 定义 |
| 提交平台结果 | Worker 幂等提交成功、失败或未知结果 | 待 IDL 定义 |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| PublishTask | PublishAttempt | 单次发布意图及尝试 | 终态不可覆盖；同一 attempt 只绑定一个 platform_request_id |

- `user_id + asset_id + target + idempotency_key` 唯一。
- `asset_id`、`workspace_id`、`target` 创建后不可改变。
- `result_unknown` 必须先查询平台结果，不能直接重新发布。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| VideoPublished | Task 进入 succeeded | task_id, asset_id, target, platform_publish_id, publish_url | 调用方、Notification |
| VideoPublishFailed | Task 进入 failed | task_id, error_class | 调用方、Notification |
| VideoPublishResultUnknown | 平台结果无法确定 | task_id, platform_request_id | 协调 Worker、运维 |

## 数据模型

### `publish_tasks`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Publish Task ID |
| workspace_id | text | Workspace ID |
| user_id | text | 发起用户 |
| asset_id | text | VIDEO Asset ID，仅逻辑引用 |
| object_path_snapshot | text | 提交时解析的视频对象路径快照 |
| target | text | 发布目标 |
| metadata | jsonb | 标题、标签等平台参数 |
| idempotency_key | text | 调用方稳定幂等键 |
| status | text | pending/publishing/succeeded/failed/result_unknown/cancelled |
| attempt | integer | 当前尝试编号 |
| platform_request_id | text nullable | 平台侧稳定请求标识 |
| platform_publish_id | text nullable | 平台发布 ID |
| publish_url | text nullable | 发布成功 URL |
| error_class | text nullable | 稳定错误分类 |
| error_detail | text nullable | 脱敏错误详情 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

#### 约束与索引

- `PRIMARY KEY (id)`
- `UNIQUE (user_id, asset_id, target, idempotency_key)`
- `UNIQUE (target, platform_request_id)`，允许 NULL。
- 按 `status, updated_at` 建协调任务索引。

## 能力详细说明

### 创建发布任务

1. 使用可信 Principal 调 Materials 解析 Asset，要求 Workspace 一致、用户可访问且媒体类型为 VIDEO。
2. 生成稳定 `platform_request_id`，创建 pending Task 与 Outbox。
3. 命中业务唯一键时返回既有 Task。

### 提交平台结果

1. 按 `task_id + attempt + expected_status` compare-and-set。
2. 成功保存 platform_publish_id/URL；永久错误保存脱敏分类；超时且无法查询时进入 result_unknown。
3. cancelled 或其他终态收到晚到结果时只记录审计，不覆盖终态。

## 关联关系

### 依赖的 Domain
- `media_materials/asset` - 解析 Asset、媒体类型和访问权。

### 被依赖的 Domain
- 无。

平台发布 Adapter 位于 media_gen 基础设施层，依赖 Publish 定义的命令与结果契约，但不是独立 Domain。

### 参与的 Workflow
- [video-publish](../../workflow/0002-video-publish/video-publish.md) - 协调平台调用和结果恢复。

### 关联术语
- `Publish Task`
- `Publish Target`
- `Publish State`
