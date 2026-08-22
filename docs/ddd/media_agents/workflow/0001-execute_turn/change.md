# Execute Turn 变更记录

## 2026-08-22 · TX-ENQUEUE 与首次 TX-CLAIM 落地

- Agent HTTP API 接受幂等 SubmitTurn，并原子持久化 queued Turn 与 user Message。
- 无 active Turn 时在同一调度临界区声明队首 running；已有 active Turn 时保持 FIFO queued。
- 并发、重复提交、事务回滚与真实 HTTP media-cli E2E 已覆盖；Runtime 执行与终态留在下一切片。

## 2026-08-22 · 执行事实库统一为 PostgreSQL

- TX-ENQUEUE、TX-CLAIM、TX-EVENT、TX-SNAPSHOT 与 TX-FINAL 均提交到 Agent 自有 PostgreSQL。
- 事务、事件发布与 Gateway 补查语义不变；只修订数据库产品选择。

## 2026-08-18 · 从 Gateway 接管完整执行闭环

- Agent 先持久化 Queued Turn，再按 Runtime Session FIFO 串行声明 running Turn。
- Agent 负责 Runtime 输出分级、Message/Universal Event 持久化、Session sequence 和终态 flush。
- 事件提交后发布 Session Event Bus；发布失败由 Gateway 持久化回放和补查恢复，不回滚执行事实。
- failed/interrupted 后暂停自动排空；自然 completed 才自动拉起下一 queued Turn。
