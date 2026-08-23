# Agent Session 配置切换实现规格

## 范围

当 Workspace 的 Agent Type 或 Provider API Key 实际变化并递增 `configuration_version` 时，既有 Agent Session 不原地迁移。UI 自动创建并选择一个绑定新版本的 Session；旧 Session 保留历史读取、事件订阅和 Interrupt，但提交新 Turn 返回 HTTP 409。

## API 契约

- `SessionResponse` 增加 `int64 context_version`，来源是 Gateway Registration 固定字段。
- 配置不兼容的新 Turn 返回 `409`，错误为 `agent session configuration changed; create a new session`。
- List Message/Event/Agent Request 与 Interrupt 不读取 Workspace 当前 Runtime Context，按 Registration 固定 Agent Type 路由。
- Workspace Response 的 `configuration_version` 仍是当前配置事实；API Key 不回显。

## UI 行为

1. 保存 Workspace 前记录旧 `configuration_version`；响应版本未改变时只刷新 Workspace。
2. 响应版本改变时调用 `CreateSession(workspace_id)` 并选中返回 Session。
3. 创建结果不明确时重新 `ListSessions`，从中选择 `context_version == workspace.configuration_version` 的最新 Session。
4. 当前 Session 版本不匹配时隐藏发送表单，显示只读说明和 `Start new session` 按钮；运行中 Session 仍显示 Stop。
5. 新 Session 选中后旧 SSE subscription 必须被取消，沿用现有 request version/active session 防晚到写入。

## Harness 映射

| 能力 | 位置 |
|:---|:---|
| Session 不变量和类型化冲突 | `media_agent_gateway/biz/domain/agent_session` |
| 新 Turn 兼容校验、旧 Turn Interrupt | `media_agent_gateway/biz/workflow/run_turn.go` |
| HTTP 状态与 Session 投影 | `media_agent_gateway/biz/handler/chat` |
| API 唯一来源 | `idl/media_agent_gateway/chat.proto` + 生成 model/client/OpenAPI |
| UI 配置切换协调 | `ui_platform/src/pages/chat/index.tsx` |
| 前端 API/类型 | `ui_platform/src/domains/chat`、`domains/workspace` |

## 验收矩阵

| 原能力/场景 | 目标上下文 | Domain/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|
| 更新 Workspace Agent/API Key | media_control | AgentConfiguration | agent_configuration_switch | covered |
| 为新版本创建 Session | media_agent_gateway/media_agents | Registration/Runtime Session | agent_configuration_switch + startup | covered |
| 旧 Session 提交新 Turn | media_agent_gateway | Registration compatibility | run_turn | covered |
| 旧 Session 历史/事件读取 | media_agents，经 Gateway | Runtime Session | 无：授权只读代理 | covered |
| 配置切换前 running Turn 中断 | media_agents，经 Gateway | Runtime Session | run_turn | covered |
| UI 自动切换与失败恢复 | ui_platform | 当前 Workspace/Session URL 状态 | agent_configuration_switch | covered |

验收包含 Gateway workflow/handler 单测、UI 单元与 Playwright、Go HTTP E2E，以及真实 `media-cli` Workspace 更新→新 Session→Turn 操作链路。
