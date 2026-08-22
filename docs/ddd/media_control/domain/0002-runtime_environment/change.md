# Runtime Environment 变更记录

## 2026-08-22 · 增加 Workspace Runtime Directory

- Placement 增加 Workspace 独占的持久化运行目录引用。
- Agent 容器内固定使用 `/workspace`，本地 Docker adapter 映射宿主 `/workspace/<workspace_id>`。
- stop/restart 保留目录；mount identity 不一致时拒绝启动，禁止跨 Workspace 文件串用。
