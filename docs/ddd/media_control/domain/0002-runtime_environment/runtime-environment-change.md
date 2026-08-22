# Runtime Environment 变更记录

## 2026-08-22：环境、Placement 与运行目录绑定

- 新增 Runtime Environment 的生命周期和容量 JSON 配置。
- 新增 Workspace Placement 对账，active 环境优先选择；无可用环境持久化 `unplaced` 原因。
- Placement 固定生成 `/workspace/<workspace_id>` volume reference，供 Runtime Controller adapter 使用；本切片不启动 Runtime。
- Environment Grant/Token 留待后续独立切片。
