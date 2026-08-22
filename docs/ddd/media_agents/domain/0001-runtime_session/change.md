# Runtime Session 变更记录

## 2026-08-22 · Execute Turn 终态与暂停规则落地

- running Turn 可原子进入 completed、failed 或 interrupted，终态不可覆盖。
- completed 在同一调度事务声明 FIFO 队首；failed/interrupted 把 Runtime Session 置为 paused 并保留 queued Turn。
- 增加 Turn 查询和软中断入口，Runtime Handle 随首次执行持久化。

## 2026-08-22 · 区分业务 Session 与 Runtime handle

- 新增可空 `runtime_session_id`，持久化 ACP/Codex 或 Claude Code 返回的底层会话句柄。
- 后续 Turn 与重启恢复使用该句柄；禁止假设它等于 Gateway 分配的业务 `session_id`。
- API key 等秘密通过 write-only 执行信封进入 workflow，只在对应 Turn 的内存执行上下文存在，不进入 Turn input、Message 或 Runtime Session 表；恢复时再按 `context_ref` 协调重取。

## 2026-08-22 · Submit Turn 持久化 FIFO 落地

- 使用 Session 行锁分配严格递增 enqueue sequence，并以 partial unique index 保证至多一个 running Turn。
- `session_id + idempotency_key` 重试返回既有 Turn，不重复写 user Message。
- Turn、user Message、Session 计数器和首条 Turn 声明在同一 PostgreSQL 事务内提交。

## 2026-08-22 · Agent 事实库统一为 PostgreSQL

- Runtime Session、Turn、Message、Universal Event 与 Agent Request 使用 Agent 自有 PostgreSQL database/schema。
- 数据库产品统一不改变 Agent 的事实所有权；Gateway 仍禁止直连并只能使用 Agent API。
- running Turn 唯一约束使用 PostgreSQL partial unique index 实现。

## 2026-08-18 · 接收 Gateway 的 Turn 与事件所有权

- Runtime Session 成为 Turn、Message、Universal Event、Session sequence、Chat Status 和 Agent Request 的唯一事实所有者。
- `pending_message` 单槽位改为持久化 Queued Turn；同一 Session 使用 FIFO 队列和 running 唯一约束串行执行。
- Universal Event 先提交 Agent 数据库，再发布 Session Event Bus；Gateway 通过 Agent 查询接口回放，不直连数据库。
- Gateway 节点、浏览器 SSE 连接和消息中间件 offset 均不参与 Turn 执行所有权判断。
