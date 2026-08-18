# Session Analytics 变更记录

## 2026-08-18 · Turn 事实来源迁移到 Agent

- Session/Task 元数据仍来自 Gateway，Turn、Message 与 Usage 事实改为消费 Agent 终态事件。
- 总线只承担通知；漏投或投影重建通过 Agent Replay API 按 Session sequence 完成，禁止共享 Agent 数据库。
