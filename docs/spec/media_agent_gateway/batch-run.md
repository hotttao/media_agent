# Batch Run 实现规格

## 边界

Batch Run 冻结一组 `item_key + session_id + input_json`，异步拆分为独立 Agent Task，并只保存 Item/计数投影。Agent Task 仍是单任务事实源，Agent 仍拥有 Turn、FIFO、Message 与 Universal Event。

## 能力覆盖矩阵

| 设计能力 | Domain/Workflow | 当前状态 |
|:---|:---|:---|
| Create/Get/List/Cancel Batch 与冻结 Item | batch_run | covered |
| `(requested_by, workspace_id, idempotency_key)` 幂等创建 | batch_run | covered |
| Session owner/workspace 与 JSON object 校验 | batch_run | covered |
| 稳定游标拆分为幂等 Agent Task | batch_run_dispatch | covered |
| Agent Task 终态 Inbox 去重与 Item/计数投影 | batch_run_dispatch | covered |
| Batch completed/partial_failed/failed/cancelled 收敛 | batch_run_dispatch | covered |
| 取消向未终态 Agent Task 传播 | batch_run_dispatch | covered |

只要矩阵仍有 `missing`，Batch Run 不得声明完成。

## HTTP API

- `POST /api/v1/batch-runs`：冻结 1..1000 个 Item；相同幂等键返回既有 Batch。
- `GET /api/v1/batch-runs?workspace_id=...`：仅列出调用者 Batch。
- `GET /api/v1/batch-runs/:batch_run_id`：返回 Batch 与固定 Item 投影。
- `POST /api/v1/batch-runs/:batch_run_id/cancel`：原子记录取消，并由 Worker 向已关联的未终态 Agent Task 传播。

Item key 在 Batch 内唯一，`input_json` 必须为 JSON object。响应不得包含 Cookie、Session Token、Provider API Key 或其他凭据。

## Harness 映射

- IDL：`idl/media_agent_gateway/batch_run.proto`
- Domain：`media_agent_gateway/biz/domain/batch_run`
- Repository：`media_agent_gateway/biz/dal/repo/batch_run`
- Service：`media_agent_gateway/biz/service/batch_run`
- Workflow：`media_agent_gateway/biz/workflow/batch_run_dispatch`
- Migration：`media_agent_gateway/migrations/000007_batch_runs.*.sql`、`000008_batch_run_dispatch.*.sql`
- E2E：`media-cli/e2e/media_agent_gateway_batch_run_e2e_test.go`
