# Execute Turn 变更记录

## 2026-08-22 · Codex ACP 与 Claude Code 执行编排落地

- 两种 Runtime 通过统一 port 接入 Execute Turn Flow，所有外部调用均位于数据库事务外。
- workflow 在后台串行排空 completed 队列；Runtime 失败和确认中断提交稳定终态并暂停队列。
- SubmitTurn write-only 信封支持 API key/Session Token，秘密不进入 input、响应、日志或 PostgreSQL，失败/中断后清除内存副本。

## 2026-08-22 · 增加 Runtime handle 建立步骤

- 首个 Turn 创建底层 Runtime 会话并持久化返回句柄；后续 Turn 加载该句柄。
- Runtime 调用保持在数据库事务外，只有句柄和终态属于 Agent 事务事实。
- 执行凭据通过 write-only 信封传入并仅在 Turn 内存上下文暂存，禁止随持久化 input 传入；重启恢复按版本化 `context_ref` 重新协调。

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
