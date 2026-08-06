---
name: notification
description: 用户可见通知与读取状态
dependencies: []
related_workflow:
    - notification_delivery
---

## 一句话职责

Notification 负责把幂等业务通知意图保存为用户可查询、可标记已读的通知。

## 边界

### 做什么
- 幂等创建 Notification。
- 查询用户通知并维护已读状态。
- 保存用户 Notification Preference。

### 不做什么
- 不拥有触发通知的业务事实。
- 不执行外部渠道投递，投递归 Delivery。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建通知 | 从业务请求生成用户通知 | `NotificationInternalService.CreateNotification` |
| 查询通知 | 分页读取当前用户通知 | `NotificationService.List/Get` |
| 标记已读 | 更新单条或全部读取状态 | `MarkRead/MarkAllRead` |
| 管理偏好 | 维护类型与渠道选择 | `NotificationPreferenceService.Get/Update` |

## 数据模型

### `notifications`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Notification ID |
| user_id | text | 接收用户 ID |
| workspace_id | text nullable | 关联 Workspace |
| type | text | 通知类型 |
| source | text | 产生通知的服务 |
| source_resource_id | text | 来源业务记录 ID |
| title | text | 通知标题 |
| body | text | 通知正文 |
| read_at | timestamptz nullable | 用户读取时间 |
| created_at | timestamptz | 创建时间 |

### `notification_preferences`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| user_id | text | 用户 ID |
| type | text | 通知类型 |
| channel | text | 站内、企微等渠道；三者组成联合主键 |
| enabled | boolean | 是否允许该渠道投递 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Notification 使用 ULID；重复业务请求返回已有记录。

## 能力详细说明

### CreateNotification

1. 校验来源服务 allowlist 与通知类型。
2. 按业务幂等键 upsert Notification。
3. 根据 Preference 创建 Delivery 请求与 Outbox，不在事务内调用 Channel。
