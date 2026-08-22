# Batch Run 变更记录

## 2026-08-22 · 对齐 Workflow 与 Agent Task

- Batch 状态机统一为 `creating/running/completed/partial_failed/failed/cancelled`。
- 每个冻结 Item 明确保存 `session_id`；创建时校验 Session owner 与 workspace，拆分 Worker 不再猜测执行 Session。
- Batch/Item ID 使用 UUID v4；`batch_run_id + item_key` 映射为 Agent Task 的稳定来源与幂等键。
- P1 管理切片与 P2 Task 拆分/终态投影分开提交。

## 2026-08-22 · Batch Run 管理切片

- 增加 Batch Run 幂等创建、owner 查询、冻结 Item 与 creating 状态取消。
- Batch、Item 与计数在同一 PostgreSQL 事务更新；P1 不提前创建 Agent Task。
- Task 拆分、terminal Inbox 投影和 running 取消传播仍归属 P2。

## 2026-08-22 · Task 拆分与终态投影

- 创建 Batch 时同事务写入 Dispatch Outbox，River Worker 按 `next_cursor` 逐项创建幂等 Agent Task，并以 CAS 保存 `agent_task_id`。
- Agent Task 终态通过其 Outbox 投影到 Batch Inbox；`event_id` 去重后重算 Item 终态计数并收敛 Batch 状态。
- running Batch 取消后，未关联 Item 直接取消，已关联且未终态的 Agent Task 由取消 Job 传播取消意图。
- 增加重复终态、游标恢复和取消传播单测，以及真实容器 media-cli E2E。
