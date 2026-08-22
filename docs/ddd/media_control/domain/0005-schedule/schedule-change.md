# Schedule 变更记录

## 2026-08-22：Schedule 管理与立即触发

- 新增 Workspace-owned Agent Schedule 与 Schedule Trigger 持久化。
- Schedule 写操作使用 owner 校验和 `expected_version`；RunNow 创建 pending Trigger，不执行 Agent Task。
- 到期领取、Task 提交和调度 worker 留待后续 agent schedule execution workflow。
