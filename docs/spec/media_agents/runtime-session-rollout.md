# Agent Runtime Session 落地规格

## 范围

本规格把 `media_agents/runtime_session` 与 `execute_turn` 的目标设计映射到 Python
Agent Harness。Gateway 到 Agent 的切换按可独立验收的垂直能力推进；每个阶段必须同时
具备领域测试、应用/DAL 测试和 `media-cli` HTTP E2E。`media_workflow` 不在范围内。

## Harness 映射

| 设计对象 | 实现位置 |
|:---|:---|
| Runtime Session 聚合、Turn、Message、Universal Event | `media_agents/adapter/biz/domain/runtime_session/` |
| 单 Domain 命令 | `media_agents/adapter/biz/service/runtime_session/` |
| Execute Turn 跨 Runtime/事件总线流程 | `media_agents/adapter/biz/workflow/execute_turn_flow.py` |
| SQLAlchemy 行模型与仓储 | `media_agents/adapter/biz/dal/models/`、`repos/runtime_session/` |
| 事务与数据库装配 | `media_agents/adapter/biz/dal/db.py`、`session.py` |
| HTTP DTO/OpenAPI/路由 | `media_agents/adapter/schemas/runtime_session/`、`idl/runtime_session.yaml`、`biz/api/v1/routes/runtime_session/` |
| HTTP E2E 驱动 | `media-cli/cmd/media_agents/`、`media-cli/e2e/` |

Domain、service、workflow 不依赖 FastAPI、Pydantic 或 SQLAlchemy。事务只由 service 或
workflow 顶层开启，repository 从 context-local session 取得连接。

## 能力覆盖矩阵

| 原能力/设计能力 | 目标限界上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| Gateway Session Registration | media_agent_gateway | agent_session | Agent Session Registration | agent_session_startup | external-owner |
| 注册 Runtime Session | media_agents | runtime_session | Runtime Session | agent_session_startup | covered |
| 幂等提交并排队 Turn | media_agents | runtime_session | Runtime Session / Turn Queue | execute_turn | covered |
| 串行声明 queued Turn | media_agents | runtime_session | Runtime Session / Turn | execute_turn | covered |
| 执行 Runtime Turn | media_agents | runtime_session | Turn | execute_turn | covered |
| 持久化 Message | media_agents | runtime_session | Message | execute_turn | covered |
| 持久化 Universal Event 与 Session sequence | media_agents | runtime_session | Universal Event | execute_turn | covered |
| 发布 Live/Universal Event | media_agents | runtime_session | Session Event Bus | execute_turn | covered |
| 回放 Universal Event | media_agents | runtime_session | Universal Event 读模型 | execute_turn | covered |
| 查询 Message 与 Chat Status | media_agents | runtime_session | Message / Runtime Session | execute_turn | covered |
| 软中断当前 Turn | media_agents | runtime_session | Turn | execute_turn | covered |
| 解决 Agent Request | media_agents | runtime_session | Agent Request | execute_turn | covered |
| Gateway Web 鉴权与 SSE framing | media_agent_gateway | agent_session | Registration/SSE adapter | run_turn | external-owner |
| Gateway 直接持久化交互事件 | media_agent_gateway | agent_session | 旧 Message/Event 写模型 | run_turn | deprecated-with-reason |

补充说明：Runtime Turn 已覆盖 Codex ACP 与 Claude Code；两个 Gateway 能力由
`media_agent_gateway` 拥有。Gateway 直接持久化交互事件依据 ADR-0009 废弃，P5 已删除运行时
写入口，并提供双库历史回填、验证与显式清理工具。

在所有 `missing` 被实现或明确移交前，本迁移不得宣称完成。

## 垂直切片

### 1. 注册 Runtime Session

提供幂等注册命令。请求包含 Gateway 分配的 `session_id`、正整数
`context_version` 和版本化运行上下文引用；同 ID 同版本重复注册返回既有聚合，不同版本
冲突。提交 `runtime_sessions` 后才响应。该切片同时建立数据库迁移、事务抽象、repository、
依赖注入和 `media-cli media_agents runtime-session register/get`。

### 2. Submit Turn 与持久化 FIFO

实现 `session_id + idempotency_key` 去重、enqueue sequence、queued Turn 与 user Message
原子提交，并在无 running Turn 时声明队首。数据库约束保证每 Session 最多一个 running。

### 3. Execute Turn 与终态

把现有 ACP `run_turn` 和 Claude Code CLI 放入统一的 Runtime port。首次执行创建并持久化
`runtime_session_id` Runtime Handle，后续执行加载该句柄；Runtime 调用不持有数据库事务。
API key 等秘密放在 SubmitTurn 的 write-only 执行信封中，仅在对应 Turn 的内存上下文暂存，
禁止进入 Turn input、响应、日志或持久化表；失败/中断清除，P6 恢复按 `context_ref` 重新协调。
自然完成自动声明下一 queued Turn，failed/interrupted 暂停队列。

### 4. Universal Event、Message 与回放

实现 Agent-owned sequence、完成 Message、tool result 聚合/截断、持久化后发布，以及
`after_sequence` 回放。Runtime Session 使用独立 `next_event_sequence` 与
`next_message_sequence`，分别服务事件恢复和 Message 查询，禁止 `MAX+1`。Live frame 不分配 sequence。

### 5. Gateway 切换与旧写路径移除

Gateway 改为命令/查询代理和 SSE framing，订阅总线前先注册接收者，再回放并按 sequence
去重。跨节点、发布失败补查和重连通过 E2E 后，删除 Gateway 旧交互事实表写入口。

### 6. Agent Request 与恢复

持久化请求/响应；启动扫描 running Turn，与 adapter 协调后重新挂接、安全恢复或标记
failed，禁止在结果不明确时重复不可逆副作用。

## 数据库

按 ADR-0010 使用 Agent 自有 PostgreSQL database/schema、SQLAlchemy asyncpg 与 Alembic。
数据库产品统一不合并限界上下文：Gateway 不获得表访问权，只通过 Agent API 查询和回放。

## 质量门禁

每个切片运行 `ruff check`、`ruff format --check`、`mypy`、`lint-imports`、`vulture`、
`pytest`，并通过真实 HTTP 的 `media-cli` E2E。涉及 Gateway 的切片同时运行 Go
`make arch-check`/相关测试。每个切片单独提交，不把多个未验收能力揉成一个提交。
