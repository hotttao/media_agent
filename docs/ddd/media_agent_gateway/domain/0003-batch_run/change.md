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
