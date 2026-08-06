# 005 异步任务与事件规范

## 1. River 的定位

各服务使用 River + PostgreSQL 执行本服务拥有的可靠后台任务。River 是任务执行适配器，不是服务间 Transport，也不是领域模型。

| 服务 | 典型任务 |
|---|---|
| `media_agent_gateway` | Agent Task、Batch Run 分发与重试 |
| `media_control` | Schedule 触发、配置协调任务 |
| `media_notification` | 渠道投递、重试、死信处理 |
| `media_storage` | 回收期清理、完整性检查 |

任务载荷只保存业务 ID、幂等键和必要参数，不保存 Cookie、Session Token、Platform Token 或用户 OAuth Token。

## 2. 跨服务一致性

- 本地状态变更与 Outbox 记录在同一数据库事务提交。
- 发布方异步投递集成事件；消费方使用 Inbox 或业务幂等键去重。
- 语义为至少一次投递，消费者不得依赖恰好一次。
- 首期可使用内部 HTTP 投递；当吞吐、扇出或隔离需求出现时，可替换为 NATS，不改变领域事件和 Outbox/Inbox 模型。

典型事件：`WorkspaceChanged`、`AgentConfigChanged`、`PluginBindingChanged`、`MaterialDeleted`、`NotificationRequested`。

## 3. Schedule

1. `media_control` 保存 Schedule 与执行主体。
2. River 到期任务加载最新配置并创建一次执行记录。
3. 需要代表用户发起离线任务时，Worker 即时向 `media_auth` 获取受限用途的 Platform Token。
4. Worker 调用 `media_agent_gateway` 创建 Agent Task；Token 不写入任务表或事件。

Schedule 是控制面聚合；Schedule Worker 是其后台执行组件，不是独立服务。

## 4. 失败规则

- 不在持有数据库事务时发起外部 RPC。
- 每种任务声明幂等键、最大重试次数、退避策略和不可重试错误。
- 超过重试上限进入失败或死信状态，保留可审计原因。
- 消费者先完成授权与状态前置检查，再执行副作用。
