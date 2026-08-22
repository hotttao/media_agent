# Agent Task 实现规格

## 边界

Agent Task 是 Gateway 持有的异步执行请求。Gateway 保存请求身份、输入快照、幂等键和状态；Agent 持有 Turn、队列顺序、Message 与 Universal Event。River/Outbox 只负责至少一次投递，不是业务事实源。

## 能力覆盖矩阵

| 原能力/设计能力 | 目标上下文 | Domain/聚合 | Workflow | 当前状态 |
|:---|:---|:---|:---|:---|
| Create/Get/List/Cancel Task | media_agent_gateway | agent_task/AgentTask | 无（单聚合） | covered |
| 幂等创建 `(requested_by, source, source_resource_id, idempotency_key)` | media_agent_gateway | agent_task/AgentTask | 无 | covered |
| queued/retry_wait 原子领取 | media_agent_gateway | agent_task/AgentTask | agent_task_dispatch | missing |
| 向 Agent SubmitTurn，`task_id` 作为幂等键 | media_agents | runtime_session/Turn | agent_task_dispatch | covered（Agent API 已存在） |
| 保存 turn_id 与 waiting_turn | media_agent_gateway | agent_task/AgentTask | agent_task_dispatch | missing |
| Turn 终态收敛与漏投补偿查询 | media_agent_gateway + media_agents | AgentTask + Turn | agent_task_dispatch | missing |
| Agent FIFO、Message、Universal Event 持久化 | media_agents | runtime_session | execute_turn | covered |
| River Job 投递 | 基础设施 | 无 | agent_task_dispatch | missing |

只要矩阵仍有 `missing`，Agent Task 设计不得声明完成。

## 数据模型

`agent_tasks` 包含：id、workspace_id、requested_by、可选 session_id、source、source_resource_id、idempotency_key、JSONB input、status、attempt、可选 turn_id/result_ref/error_code/error_message、created_at/started_at/finished_at/updated_at。终态为 completed/failed/cancelled。

管理切片先实现表、单聚合命令与 HTTP API；Dispatch 切片增加 Outbox/River adapter、认领、SubmitTurn 和终态收敛。

## HTTP 接口

- `POST /api/v1/agent-tasks`：按幂等键创建或返回既有 Task。
- `GET /api/v1/agent-tasks?workspace_id=...`：仅查询调用者任务。
- `GET /api/v1/agent-tasks/:task_id`：仅查询调用者任务。
- `POST /api/v1/agent-tasks/:task_id/cancel`：queued/retry_wait 直接取消；running/waiting_turn 记录取消意图，Dispatch Workflow 调 Agent 取消/中断并收敛。

响应不得包含 Cookie、Session Token、Platform Token、Provider API Key 或其他凭据。`input_json` 必须是 JSON object。

## 状态与并发

- 创建初态 queued，attempt=0。
- 管理切片允许 queued/retry_wait -> cancelled；重复取消返回当前 cancelled。
- Dispatch 通过状态 CAS：queued/retry_wait -> running -> waiting_turn -> terminal。
- task_id 作为 Agent SubmitTurn 幂等键；一个 Task 最多一个有效 turn_id。
- 终态不可互相覆盖；晚到事件和过期 attempt no-op。

## Harness 映射

- IDL：`idl/media_agent_gateway/agent_task.proto`
- Domain：`media_agent_gateway/biz/domain/agent_task`
- Repository：`media_agent_gateway/biz/dal/repo/agent_task`
- Service：`media_agent_gateway/biz/service/agent_task`
- Handler/Router：Hertz 生成骨架与 middleware 扩展点
- Workflow：`media_agent_gateway/biz/workflow/agent_task_dispatch`
- Migration：`media_agent_gateway/migrations/000005_agent_tasks.*.sql`
- HTTP E2E：`media-cli/e2e/media_agent_gateway_agent_task_e2e_test.go`
