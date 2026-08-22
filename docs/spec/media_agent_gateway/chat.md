# media_agent_gateway 最小 Chat 实现规格

## 概述与边界

实现已登录用户进入 `ui_platform` 后立即使用所选 Agent 对话的闭环。`media_agent_gateway`
只拥有 Agent Session Registration、Web 授权和 SSE framing；`media_agents` 拥有 Runtime
Session、Turn、Message、Universal Event 与 Chat Status。`media_auth` 提供 Cookie Session
身份，`media_control` 提供 Workspace 对应的 Agent 类型、版本化上下文与执行凭据。

## 运行拓扑

```text
ui_platform --Cookie/Command+Subscription--> media_agent_gateway --Command/Replay--> media_agents
                                |                         |
                                |                         +--> PostgreSQL media_agents
                                +--> PostgreSQL Registration
                                +<------ Redis Pub/Sub Session Event Bus
```

## API

- `GET /api/chat/session`：返回当前用户最近的 active Session；不存在则用 Default Agent Context 创建。
- `GET /api/chat/sessions/{session_id}/messages`：授权后代理 Agent Message 查询。
- `POST /api/chat/sessions/{session_id}/turns`：请求体 `{message, idempotency_key}`，向 Agent 幂等提交 Turn 并返回 `queued|running`。
- `POST /api/chat/sessions/{session_id}/interrupt`：授权后读取 Agent active Turn 并调用 Agent 中断命令。
- `GET /api/chat/sessions/{session_id}/events?after_sequence=N`：先订阅 Session Event Bus，再从 Agent 回放并按 Session Sequence 去重，最后持续输出 SSE。

所有接口使用与 `media_auth` 共享的 HS256 secret 在本地验证 Cookie JWT，并从 `sub` 获取用户身份。请求体不得覆盖 `user_id`。

## 数据模型

- Gateway `agent_sessions`：`id, user_id, workspace_id, agent_id, agent_type, status, context_version, title, created_at, updated_at`。
- Agent `runtime_sessions/turns/messages/universal_events`：只通过 Agent API 访问；Gateway 不建立外键、不共享数据库连接。

创建 Gateway Registration 后使用同一 `session_id + context_version` 幂等注册 Agent Runtime
Session。Turn、Message 和事件不再写 Gateway 表。Live Event Frame 从总线尽力转发；持久事件
以 Agent Replay API 为恢复事实。Gateway 先订阅再回放，丢弃 `sequence <= cursor` 的重复，
发现缺口、终态或周期检查时从当前 cursor 补查。

## Codex 与镜像

- Runtime 使用现有 `media_agents/codex` 与 `codex-acp`。
- 构建镜像时将构建上下文中的 `.codex/auth.json` 安装到镜像运行用户的 `/root/.codex/auth.json`，权限 `0600`；构建脚本默认从 `/home/tao/.codex/auth.json` 准备该构建上下文文件。
- 不从 media_control 重写 Codex auth；内置 mock provider 返回默认 Workspace、Codex Agent、空 MCP/技能/凭证，不覆盖镜像中的 `auth.json`。

## UI

登录成功仍跳转 `/`，受保护首页渲染 Chat 页面。页面加载 Session 和历史 Message，并独立订阅 Session Event；提交后立即显示 User Message，订阅消费 `item.started/item.delta/item.completed/session.ended/error`，流式拼接 assistant 文本。streaming 时发送按钮替换为停止按钮；中断命令成功后仍等待广播终态，不乐观结束 Turn。多个页面打开同一 Session 时都接收广播，刷新和断线使用 sequence 恢复。

## 配置

- `POSTGRES_DSN`：Gateway PostgreSQL DSN。
- `JWT_SECRET`：与 media_auth 共享，local/dev 默认 `dev-shared-secret`。
- `CODEX_RUNTIME_URL`、`CLAUDE_CODE_RUNTIME_URL`：Agent Runtime Session API 地址。
- `MEDIA_AGENTS_REDIS_URL`：Session Event Bus Redis 地址；不可用时降级为周期回放。
- `MEDIA_AGENTS_EVENT_CHANNEL_PREFIX`：与 Agent 一致的 Session channel 前缀。
- `DEFAULT_WORKSPACE_ID`：默认 `default`。
- `DEFAULT_AGENT_ID`：默认 `codex`。

Gateway PostgreSQL 只保存 Registration。历史旧 Turn/Message/Event 表在回填到 Agent 后停止读取，
随后由独立 migration 删除；切换后的任何请求都不得继续写旧表。

历史回填使用 `cmd/agent-history-migrate`，显式连接 `POSTGRES_DSN` 与
`MEDIA_AGENTS_MIGRATION_DSN`，不进入 Gateway 运行时依赖：

1. `--mode import` 按 Session 把终态 Turn、Message、Universal Event 写入 Agent 单事务，
   同时续接三个 sequence counter；存在 running/queued Turn 或上下文版本冲突时拒绝迁移。
2. `--mode verify` 比较两库逐 Session 的 Turn/Message/Event 数量及 context version。
3. `--mode cleanup --confirm-drop-legacy-history` 在同次执行中再次验证后，才删除 Gateway
   的三个旧历史表。导入可重复执行；cleanup 不允许隐式执行。
