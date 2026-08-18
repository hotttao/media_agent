# Execute Turn 变更记录

## 2026-08-18 · 从 Gateway 接管完整执行闭环

- Agent 先持久化 Queued Turn，再按 Runtime Session FIFO 串行声明 running Turn。
- Agent 负责 Runtime 输出分级、Message/Universal Event 持久化、Session sequence 和终态 flush。
- 事件提交后发布 Session Event Bus；发布失败由 Gateway 持久化回放和补查恢复，不回滚执行事实。
- failed/interrupted 后暂停自动排空；自然 completed 才自动拉起下一 queued Turn。
