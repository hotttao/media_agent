# Runtime Session 变更记录

## 2026-08-22 · Agent 事实库统一为 PostgreSQL

- Runtime Session、Turn、Message、Universal Event 与 Agent Request 使用 Agent 自有 PostgreSQL database/schema。
- 数据库产品统一不改变 Agent 的事实所有权；Gateway 仍禁止直连并只能使用 Agent API。
- running Turn 唯一约束使用 PostgreSQL partial unique index 实现。

## 2026-08-18 · 接收 Gateway 的 Turn 与事件所有权

- Runtime Session 成为 Turn、Message、Universal Event、Session sequence、Chat Status 和 Agent Request 的唯一事实所有者。
- `pending_message` 单槽位改为持久化 Queued Turn；同一 Session 使用 FIFO 队列和 running 唯一约束串行执行。
- Universal Event 先提交 Agent 数据库，再发布 Session Event Bus；Gateway 通过 Agent 查询接口回放，不直连数据库。
- Gateway 节点、浏览器 SSE 连接和消息中间件 offset 均不参与 Turn 执行所有权判断。
