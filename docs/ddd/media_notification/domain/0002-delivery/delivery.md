---
name: delivery
description: Notification 的渠道投递、重试与死信
dependencies:
    - notification
related_workflow:
    - notification_delivery
---

## 一句话职责

Delivery 负责把 Notification 投递到指定 Channel，并保存每次尝试与最终结果。

## 边界

### 做什么
- 创建每个 Notification/Channel 的 Delivery。
- 通过 River 执行退避重试。
- 达到重试上限后转入 Dead Letter。

### 不做什么
- 不改变 Notification 的业务内容或已读状态。
- 不把 River Job 当 Delivery 事实源。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 入队投递 | 创建幂等 Delivery | `NotificationInternalService.EnqueueDelivery` |
| 执行投递 | 调用 Channel 并记录 attempt | `DispatchDelivery` |
| 重试投递 | 按错误类型和预算重新调度 | `RetryDelivery` |
| 查询死信 | 提供运维审计和重放 | `ListDeadLetters/ReplayDelivery` |

## 数据模型

### `deliveries`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Delivery ID |
| notification_id | text | 所属 Notification |
| channel | text | 目标通知渠道 |
| destination_ref | text | 渠道内目标用户或地址引用 |
| status | text | pending/delivering/delivered/retry_wait/dead_letter/cancelled |
| attempt_count | integer | 已执行次数 |
| next_attempt_at | timestamptz nullable | 下一次重试时间 |
| last_error_code | text nullable | 最近稳定错误码 |
| delivered_at | timestamptz nullable | 成功投递时间 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Delivery 使用 ULID；attempt 是同一 Delivery 的递增记录，不生成新 Delivery。

## 能力详细说明

### DispatchDelivery

1. 原子领取 pending/retry_wait Delivery 并递增 attempt。
2. 事务外调用 Channel Gateway。
3. 成功标记 delivered；可重试错误计算退避；永久错误或超预算进入 dead_letter。
