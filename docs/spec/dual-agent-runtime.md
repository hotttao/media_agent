# Codex / Claude Code 双 Agent Runtime 实现规格

## 范围与契约

Workspace 可选择 `codex` 或 `claude-code`。`media_control` 是 Agent Type 与 Provider API Key 的事实源；
`media_agent_gateway` 按 Session 固定的 Agent Type 选择 Runtime；两个 Agent 容器共享同一 Workspace
Runtime Directory，但部署层同一时刻只启用所选 Runtime 写入。

公开 Workspace API 接受 `agent_type` 与可选 `api_key`，响应只返回 `api_key_configured`。
更新请求中空 Key 保留原值。内部 Runtime Context 要求用户 Cookie 与
`X-Control-Service-Token` 双重校验后才返回明文 Key，仅供 Gateway 单次 `/chat` 调用，禁止记录请求体。
支持枚举必须由后端和前端共享语义：`codex | claude-code`。

## Harness 映射

| 能力 | Harness 位置 |
|:---|:---|
| Agent Configuration 聚合与校验 | `media_control/biz/domain/workspace`、`biz/service/workspace` |
| 密钥认证加密 | `media_control/biz/shared/secret`、PostgreSQL `agent_configurations` |
| HTTP 契约 | `idl/media_control/workspace.proto` → 生成 DTO/路由 → 轻量 handler |
| Runtime 选择 | `media_agent_gateway/biz/shared/client`，Turn workflow 只依赖 `Runtime` |
| Claude CLI 生命周期 | `media_agents/claude_code`，共享 adapter 仅提供通用配置/事件 DTO |
| Workspace 表单 | `ui_platform/domains/workspace` 维护类型/客户端，页面只组合交互 |
| 黑盒验收 | `media-cli` 命令与 `media-cli/e2e` |

## Claude Code Agent

镜像安装官方 `@anthropic-ai/claude-code` CLI，固定 `WORKSPACE_DIR=/workspace`、
`HOME=/workspace/.home`，原生会话写入 `.home/.claude`。`POST /chat` 每次启动一个受管理子进程：

```text
claude --print --verbose --output-format stream-json --session-id <uuid> <message>
claude --print --verbose --output-format stream-json --resume <session_id> <message>
```

API Key 仅通过子进程环境 `ANTHROPIC_API_KEY` 注入。Agent 将 Claude stream-json 转为 Universal Event：
`session.started`、assistant `item.started/item.delta/item.completed`、工具事件、`error`、
`session.ended`。进程表以 session_id 为键；同一 session 同时只能有一个 Turn；interrupt 向进程发送
TERM，超时后 KILL，并产生 `interrupted` 终态。HTTP 断开取消当前流但不泄漏子进程。

测试用可执行 fixture 输出合法 stream-json，不调用真实 Provider；Docker 验收至少验证镜像内
`claude --version`、健康接口、HOME/Workspace 挂载和假 Provider/CLI 驱动的完整持久化事件链。

## Gateway

通用 HTTP Runtime client 复用 `/chat` 与 `/sessions/{id}/interrupt` 契约。环境变量：

- `CODEX_RUNTIME_URL`，默认 `http://127.0.0.1:3001`；
- `CLAUDE_CODE_RUNTIME_URL`，默认 `http://127.0.0.1:3003`。
- `CONTROL_INTERNAL_TOKEN`：Gateway 与 Control 共享的高熵服务令牌；进程配置无默认值，Compose 仅为
  本地开发提供显式默认值。

Registry 必须注册两个枚举。未知 Agent Type 在已提交 Turn 后明确标记失败，不回退到 Codex。

## UI

Workspace Dialog 使用受控 select，默认 Codex，编辑时加载当前 Agent Type。API Key 使用 password input，
不进入 URL、localStorage、日志或响应；关闭 Dialog 清空。聊天输入的 accessible label 和 placeholder 跟随
当前 Workspace Agent Type，设置页只显示“Configured/Not configured”。

## 验收

- Control 创建、更新、列表、Runtime Context 覆盖两个 Agent，非法枚举拒绝；
- 密钥密文存 PostgreSQL，公开 API/日志无明文，更新留空保留；
- Gateway 单测证明按 Session Agent Type 路由；
- Claude fixture E2E 产生并持久化完整 assistant message，interrupt 可终止；
- media-cli HTTP E2E 创建 Claude Workspace、验证不回显 Key、更新保留 Key并读取内部上下文；
- UI 单元/组件测试覆盖默认、编辑、切换、失败和关闭清密钥；
- Go/Python/TypeScript/Harness 架构门禁通过。
