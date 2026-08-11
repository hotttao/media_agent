# media_agent_gateway 最小 Chat 实现规格

## 概述与边界

实现已登录用户进入 `ui_platform` 后立即使用 Codex 对话的最小闭环。`media_agent_gateway` 拥有 Agent Session、Turn、Message 与 Universal Event；`media_agents/codex` 只负责 ACP Runtime 执行。`media_auth` 提供 Cookie Session 身份，`media_control` 在本阶段由内置 provider mock 合理的默认 Workspace 与 Codex Agent 配置。

## 运行拓扑

```text
ui_platform --Cookie/Command+Subscription--> media_agent_gateway --HTTP/SSE--> media_agents/codex
                                |
                                +----------> PostgreSQL media_agent_gateway
```

## API

- `GET /api/chat/session`：返回当前用户最近的 active Session；不存在则用 Default Agent Context 创建。
- `GET /api/chat/sessions/{session_id}/messages`：返回按 sequence 排序的完整 Message 历史。
- `POST /api/chat/sessions/{session_id}/turns`：请求体 `{message, idempotency_key}`，提交后台 Turn 并返回 `202 accepted`。
- `GET /api/chat/sessions/{session_id}/events?after_sequence=N`：回放游标之后的 Universal Event，再持续广播同一 Session 的新事件。

所有接口使用与 `media_auth` 共享的 HS256 secret 在本地验证 Cookie JWT，并从 `sub` 获取用户身份。请求体不得覆盖 `user_id`。

## 数据模型

- `agent_sessions`：`id, user_id, workspace_id, agent_id, runtime_session_id, status, created_at, updated_at`。
- `turns`：`id, session_id, idempotency_key, status, started_at, finished_at`，唯一键 `(session_id, idempotency_key)`。
- `messages`：`id, session_id, turn_id, role, content jsonb, sequence, created_at`。
- `universal_events`：`id, session_id, turn_id, sequence, type, payload jsonb, created_at`，唯一键 `(turn_id, sequence)`。

Turn 创建事务同时写入 User Message，Runtime 生命周期与提交请求分离。Runtime SSE 的每个 JSON event 单独或分批事务写入 `universal_events`；`item.completed` 中 message/tool 内容写入 `messages`。事件事务提交后才能按 Session 广播，多个订阅页面分别接收；断线按 sequence 回放。

## Codex 与镜像

- Runtime 使用现有 `media_agents/codex` 与 `codex-acp`。
- 构建镜像时将构建上下文中的 `.codex/auth.json` 安装到镜像运行用户的 `/root/.codex/auth.json`，权限 `0600`；构建脚本默认从 `/home/tao/.codex/auth.json` 准备该构建上下文文件。
- 不从 media_control 重写 Codex auth；内置 mock provider 返回默认 Workspace、Codex Agent、空 MCP/技能/凭证，不覆盖镜像中的 `auth.json`。

## UI

登录成功仍跳转 `/`，受保护首页渲染 Chat 页面。页面加载 Session 和历史 Message，并独立订阅 Session Event；提交后立即显示 User Message，订阅消费 `item.started/item.delta/item.completed/session.ended/error`，流式拼接 assistant 文本。多个页面打开同一 Session 时都接收广播，刷新和断线使用 sequence 恢复。

## 配置

- `POSTGRES_DSN`：Gateway PostgreSQL DSN。
- `JWT_SECRET`：与 media_auth 共享，local/dev 默认 `dev-shared-secret`。
- `CODEX_RUNTIME_URL`：默认 `http://127.0.0.1:3001`。
- `DEFAULT_WORKSPACE_ID`：默认 `default`。
- `DEFAULT_AGENT_ID`：默认 `codex`。

除数据库连接外，用户不需要 Workspace、Agent 或 media_control 配置。
