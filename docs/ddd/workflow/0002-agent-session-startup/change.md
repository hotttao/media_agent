# Agent Session Startup 变更记录

## 2026-08-18 · 拆分 Gateway Registration 与 Agent Runtime Session

- Gateway 继续拥有 Session 身份、访问控制与 Token Hash；Agent 新增同 ID 的 Runtime Session。
- Agent Runtime Session 注册成功后 Gateway Registration 才进入 active。
- 两侧通过幂等接口和稳定 session_id 协调，不共享数据库或跨库事务。
