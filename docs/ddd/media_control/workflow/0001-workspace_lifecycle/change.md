# Workspace Lifecycle 变更记录

## 2026-08-22 · 在启动 Agent 前协调持久化运行目录

- Runtime Controller 先幂等取得 Workspace Runtime Directory，再以 `/workspace` 挂载启动 Agent。
- Workspace ID 同时作为目录归属与 mount identity；冲突不可自动覆盖。
- 普通 stop/restart 不清理文件，rebuild/delete 的清理必须是显式生命周期语义。
