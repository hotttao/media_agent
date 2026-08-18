# Agent Task Dispatch 变更记录

## 2026-08-18 · Worker 改为向 Agent 提交 Queued Turn

- Worker 不再通过 Gateway 内部 run_turn 直接驱动 Runtime，而是使用 task_id 幂等调用 Agent SubmitTurn。
- Agent 决定 Turn queued/running 并保证同 Session FIFO；Gateway/River 不承担 Session 锁。
- Task 通过 Agent 终态事件或查询接口收敛，消息总线漏投不会导致事实丢失。
