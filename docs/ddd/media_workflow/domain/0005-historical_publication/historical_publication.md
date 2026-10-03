---
name: historical_publication
description: 保存和查询旧系统多视频发布记录，不执行剪辑或发布
dependencies:
  - execution
related_workflow:
  - historical_video_push_import
---

## 一句话职责

Historical Publication 保存 VideoPush 的来源身份、文字、原状态、媒体引用与有序视频成员，作为未来剪辑/发布设计可直接使用的历史事实。

## 边界

### 做什么

- 以来源 ID 和事实摘要幂等导入、只读查询 Historical VideoPush。
- 保留原始成员顺序、成员 Video 来源 ID 及目标已完成 Job ID。
- 非空来源媒体以目标 Object Path 和 checksum 保存，不依赖旧系统 URL。

### 不做什么

- 不定义多视频剪辑规则，不触发新发布，不重写来源状态。
- 不拥有或更新成员 Job、Generated Asset 或媒体对象字节。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 导入历史发布记录 | 校验全部成员及媒体事实后原子保存 | `ImportHistoricalVideoPush` |
| 查询历史发布记录 | 按 Workspace 读取成员及原事实 | `GetHistoricalVideoPush/ListHistoricalVideoPush` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| HistoricalVideoPush | 有序 VideoPushMember 值对象 | 一条来源发布记录的导入事务 | 成员非空且无重复；每个成员引用同一 Workspace/Owner 的已完成历史 Video Job；来源事实与媒体摘要不可变 |

VideoPushMember 保存 `ordinal`、`source_video_id`、`job_id`，不拥有 Job；同一 Video 可参与多个 Historical VideoPush。来源状态、`is_published` 和 `is_qualified` 是原系统快照，不驱动目标状态机。相同来源键的不同摘要是冲突，不作增量覆盖。

## 领域事件

无。导入历史记录不触发剪辑或发布。

## 数据模型

### `historical_video_pushes`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text | 目标记录 ID |
| workspace_id / organization_id / owner_user_id | text | 目标归属 |
| source_system / source_record_id / source_digest | text | 来源幂等身份与事实摘要 |
| product_id / virtual_ip_id / scene_id | text | 目标资源/来源场景关联快照 |
| template_name / title / content | text | 原模板名与发布文字 |
| source_status / is_qualified / is_published | text / bool / bool | 原系统发布状态快照 |
| video_object_path / thumbnail_object_path / manual_clip_object_path | text nullable | 已摄取的目标媒体引用 |
| video_sha256 / thumbnail_sha256 / manual_clip_sha256 | text nullable | 对应媒体校验摘要 |
| source_created_at / source_updated_at / imported_at | timestamptz | 原时间与导入时间分离 |

#### 约束与索引

- `PRIMARY KEY (id)`；`UNIQUE (workspace_id, source_system, source_record_id)`。
- 非空 Object Path 必须与非空 SHA-256 成对，且属于目标 Organization 命名空间。

### `historical_video_push_members`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| push_id | text | HistoricalVideoPush ID |
| ordinal | integer | 来源成员顺序，从 0 开始 |
| source_video_id | text | 源 Video ID |
| job_id | text | 对应目标终态历史 Job ID |

#### 约束与索引

- `PRIMARY KEY (push_id, ordinal)`；`UNIQUE (push_id, source_video_id)`；`job_id` 引用本库 Job，但不修改 Job 聚合。

## 能力详细说明

`ImportHistoricalVideoPush` 验证迁移身份、Workspace/Owner/Organization、成员顺序及终态 Job、媒体 Object Path 和摘要，在一个数据库事务内保存聚合。媒体摄取失败时不写记录；相同来源和相同事实摘要返回原记录，不同摘要返回冲突。Get/List 只返回当前用户有权访问的 Workspace 数据及可重新签发的目标媒体引用。

## 关联关系

### 依赖的 Domain

- [execution](../0003-execution/execution.md)：只读核验目标成员 Job。

### 被依赖的 Domain

- 无。

### 参与的 Workflow

- [historical_video_push_import](../../workflow/0004-historical_video_push_import/historical_video_push_import.md)：跨媒体摄取与本地导入的幂等编排。

### 关联术语

- Historical VideoPush、Imported Completed Execution。
