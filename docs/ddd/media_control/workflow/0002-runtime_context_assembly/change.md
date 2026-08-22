# Runtime Context Assembly 变更

## 2026-08-22：双 Agent 路由与 Provider API Key 安全下发

- Runtime Context 明确携带受控的 `agent_type`，Gateway 据此选择 Codex 或 Claude Code Runtime。
- Provider API Key 只在同时通过 Workspace owner Cookie 和 Control 服务令牌的同步内部请求中解密。
- Gateway 不持久化 Key，只随单次 Agent `/chat` 请求传入；Agent 仅注入子进程环境。
- 公开调用 Runtime Context 被拒绝，避免浏览器利用 owner Cookie 回显秘密。
