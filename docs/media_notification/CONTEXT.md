# media_notification 通知上下文

把业务服务提出的通知意图转换为用户可见通知和渠道投递，并管理偏好、重试与死信。业务事实仍归事件发布方。

## 业务术语词典

**Notification（通知）**:
面向一个用户、由业务事件派生的可读消息，拥有未读或已读状态。
_禁用替代词_：Domain Event、Delivery、Message

**Notification Preference（通知偏好）**:
用户针对通知类型和渠道配置的接收选择。
_禁用替代词_：Subscription、Channel Config

**Delivery（通知投递）**:
把一条 Notification 发送到一个 Channel 的独立尝试及状态记录。
_禁用替代词_：Notification、River Job

**Channel（通知渠道）**:
通知的目标媒介，例如站内、企微或其他 Channel Gateway 能力。
_禁用替代词_：Provider、Plugin

**Dead Letter Delivery（死信投递）**:
达到最大重试次数、需要人工或运维处理的终止投递记录。
_禁用替代词_：Failed Notification、Outbox
