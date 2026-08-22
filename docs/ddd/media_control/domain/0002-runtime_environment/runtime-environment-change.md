# Runtime Environment 变更记录

## 2026-08-22：环境、Placement 与运行目录绑定

- 新增 Runtime Environment 的生命周期和容量 JSON 配置。
- 新增 Workspace Placement 对账，active 环境优先选择；无可用环境持久化 `unplaced` 原因。
- Placement 固定生成 `/workspace/<workspace_id>` volume reference，供 Runtime Controller adapter 使用；本切片不启动 Runtime。
- Environment Grant 与连接 Token 由后续独立切片补齐。

## 2026-08-22：Runner 连接 Token

- 新增 Environment Token 签发、active 元数据列表和幂等撤销 API。
- 原始 Token 仅签发时返回，PostgreSQL 只保存 SHA-256；内部验证只产生受限 `environment_id` Principal。
- 同环境允许多枚 active Token 重叠，支持切换 Runner 后再撤销旧 Token 的无中断轮换。
- 增加摘要/验证/撤销单测和 media-cli 真实 HTTP E2E。
