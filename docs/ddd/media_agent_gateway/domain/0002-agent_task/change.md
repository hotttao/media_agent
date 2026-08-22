# Agent Task 变更记录

## 2026-08-22 · 对齐 Agent-owned Queued Turn

- Task 状态机统一为 `queued/running/waiting_turn/retry_wait/completed/failed/cancelled`。
- Task 持久化唯一 `turn_id`，使用 `task_id` 作为 Agent SubmitTurn 幂等键。
- Agent 负责同 Session FIFO 和 Turn 执行事实；Gateway Task 只维护异步请求及其收敛状态。

## 2026-08-22 · Agent Task 管理切片

- 增加 Task 创建、幂等复用、按 owner 查询与排队态取消能力。
- Session 绑定任务校验 Session owner 与 workspace；无 Session 的任务通过 Control Plane 校验 workspace 权限。
- Dispatch/Outbox、Turn 终态收敛与运行中取消仍归属后续独立切片。

## 2026-08-22 · Agent Task Dispatch/Outbox

- Task 创建与 dispatch Outbox 在同一 PostgreSQL 事务提交；Relay 幂等写入 River，Worker 使用 `task_id` 幂等提交 Agent Turn。
- Agent 接受后 Task 保存唯一 `turn_id` 并进入 waiting_turn；有界 Reconciler 以 Agent GetTurn 为权威来源收敛 completed/failed。
- 当前 Session Event Bus 是按 Session 订阅的实时通道，不作为全局 durable consumer；GetTurn 查询同时承担终态收敛和漏投补偿，后续可在不改变 Task 不变量的情况下增加事件驱动优化。
- running/waiting_turn 取消先提交 Task cancelled 与 cancel Outbox，再由可重试 River Job 中断关联 Turn。
- 取消与 SubmitTurn 竞态时，Worker 仍把 Agent 已接受的 turn_id 附加到 cancelled Task 并立即中断，避免产生未跟踪 Turn。
- Worker 通过 Control 内部服务令牌和明确 acting user ID 读取 Runtime Context，不持久化或重放用户 Cookie、Session Token 与 Provider API Key。
