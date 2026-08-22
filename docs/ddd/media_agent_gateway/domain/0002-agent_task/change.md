# Agent Task 变更记录

## 2026-08-22 · 对齐 Agent-owned Queued Turn

- Task 状态机统一为 `queued/running/waiting_turn/retry_wait/completed/failed/cancelled`。
- Task 持久化唯一 `turn_id`，使用 `task_id` 作为 Agent SubmitTurn 幂等键。
- Agent 负责同 Session FIFO 和 Turn 执行事实；Gateway Task 只维护异步请求及其收敛状态。

## 2026-08-22 · Agent Task 管理切片

- 增加 Task 创建、幂等复用、按 owner 查询与排队态取消能力。
- Session 绑定任务校验 Session owner 与 workspace；无 Session 的任务通过 Control Plane 校验 workspace 权限。
- Dispatch/Outbox、Turn 终态收敛与运行中取消仍归属后续独立切片。
