# Runtime Context Assembly 变更

## 2026-08-22：双 Agent 路由与 Provider API Key 安全下发

- Runtime Context 明确携带受控的 `agent_type`，Gateway 据此选择 Codex 或 Claude Code Runtime。
- Provider API Key 只在同时通过 Workspace owner Cookie 和 Control 服务令牌的同步内部请求中解密。
- Gateway 不持久化 Key，只随单次 Agent `/chat` 请求传入；Agent 仅注入子进程环境。
- 公开调用 Runtime Context 被拒绝，避免浏览器利用 owner Cookie 回显秘密。

## 2026-08-22：异步 Worker 服务身份

- Runtime Context 支持内部服务令牌与 `X-Control-Acting-User-ID` 组合建立代表用户身份，供不持有用户 Cookie 的 River Worker 使用。
- acting user 只在内部令牌恒定时间校验通过后生效，Assembler 仍按该用户校验 Workspace owner。
- Task、Outbox、River Job 均不保存 Cookie、Session Token 或 Provider API Key。
