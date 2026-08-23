# Teamwork Runtime Change 实现规格

## 范围

本规格覆盖 Control Teamwork Topology 变更通过事务 Outbox 可靠投递到 Gateway，以及 Gateway 对新 Agent Task 实施拓扑版本准入。Teamwork Session Link 的创建与查询不在本切片。

## Control

- `teamwork_topology_outbox` 保存 `event_id/workspace_id/topology_id/topology_version/status/event_type/payload/attempts/available_at/delivered_at/last_error/timestamps`。
- Create、Update、Delete Topology 在同一数据库事务写 `teamwork_topology.changed`；事件载荷只含 Workspace ID、Topology ID、版本和状态。
- Relay 按创建时间投递最早的未确认事件到 Gateway；非 2xx 不确认，记录失败并重试。
- 配置：`MEDIA_AGENT_GATEWAY_URL`、`CONTROL_INTERNAL_TOKEN`。

## Gateway

- 内部接口：`POST /api/v1/internal/teamwork-topology-events`，使用 `X-Control-Service-Token`。
- `teamwork_topology_versions` 保存不可变 Workspace 归属和单调 Allowed Topology Version；`teamwork_topology_inbox` 仅以 event_id 去重，重复业务版本的不同事件仍作为晚到事件记录。
- `agent_tasks` 新增可空 `topology_id/topology_version`，二者必须同时为空或同时有效。
- 创建带拓扑引用的 Task 时，Topology 必须属于 Task Workspace、为 active 且版本等于 Allowed Topology Version；否则返回 409。
- 应用更高版本事件时，同事务取消相同 topology_id 且旧版本的 queued/retry_wait Task，并为每个 Task 写 `agent_task.cancelled` Outbox。running/waiting_turn 不改变。
- 重复 event_id 幂等成功；低版本事件只记 Inbox；deleted 状态禁止该 Topology 的所有新任务。

## 能力覆盖矩阵

| 能力 | 目标上下文 | Domain/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|
| Topology Create/Update/Delete 产生可靠事件 | media_control | Teamwork Outbox | Teamwork Runtime Change | covered |
| Control 失败重试且成功后确认 | media_control | Teamwork Outbox | Teamwork Runtime Change | covered |
| Gateway 事件鉴权与 Inbox 去重 | media_agent_gateway | Teamwork Runtime Inbox | Teamwork Runtime Change | covered |
| Allowed Topology Version 单调更新 | media_agent_gateway | Teamwork Topology Version | Teamwork Runtime Change | covered |
| Task 创建版本与 Workspace 归属准入 | media_agent_gateway | Agent Task + Allowed Topology Version | 无（同步查询规则） | covered |
| 取消旧版本 queued/retry_wait Task | media_agent_gateway | Agent Task | Teamwork Runtime Change | covered |
| running/waiting_turn 自然结束 | media_agent_gateway | Agent Task | Teamwork Runtime Change | covered |
| Workspace/Topology 用户命令入口 | media-cli → media_control | Workspace + Teamwork | Teamwork Runtime Change | covered |
| Agent Task 用户命令入口 | media-cli → media_agent_gateway | Agent Task | Teamwork Runtime Change | covered |

## 验收

- Control、Gateway 单元测试与 `go test ./...` 通过。
- 两个服务 `make arch-check` 通过。
- Go HTTP E2E 证明创建事件到达、旧版本新 Task 被拒绝、旧 queued Task 被取消、重复/晚到事件不回退、内部接口拒绝普通请求。
- 独立执行真实 `media-cli` 进程链路，证明落盘 Cookie 跨服务端口复用，且 Workspace/Topology/Agent Task 命令、旧任务取消和旧版本 409 拒绝可用；该门禁不能由 Go HTTP E2E 替代。
