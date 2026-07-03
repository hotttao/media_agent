# mcp_oauth domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/mcp-oauth.ts` + `src/services/mcp-oauth.ts` + `mcp_oauth_clients` / `mcp_oauth_tokens` 表。
- **schema 修正（Q4）**：spec 原 SQL 误用单列 `refresh_failure_grace integer`，对齐 control_plane migration 117 的实际实现，改为 `refresh_fail_count integer DEFAULT 0` + `refresh_fail_first_at timestamptz`，宽限算法 = 失败≥3 且首次失败≥600s 才删 token。
- pendingAuths 内存态（10min TTL）沿用；`getValidAccessToken` 含 forceRefresh 与宽限判定逻辑不变。
- `notifyAgentReload` 回调 control_plane 仍为跨服务调用（见 mcp_oauth_connect workflow）。
