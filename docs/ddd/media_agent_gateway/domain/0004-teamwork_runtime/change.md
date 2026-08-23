# Teamwork Runtime 变更记录

## 2026-08-23：拓扑版本准入与变更 Inbox

- 新增 Allowed Topology Version 与事件 Inbox 聚合。
- Agent Task 可固定引用 `topology_id + topology_version`，创建时必须命中当前 active 允许版本。
- Allowed Topology Version 固定记录不可转移的 Workspace 归属；跨 Workspace 引用和归属变更事件均被拒绝。
- 更高版本事件取消旧版本的 queued/retry_wait Task，running/waiting_turn 保持自然结束。
- 重复或晚到事件不重复取消任务，也不会让允许版本回退。
