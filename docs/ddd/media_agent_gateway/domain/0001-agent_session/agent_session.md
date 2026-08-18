---
name: agent_session
description: Agent Session Registration、Session Token 与访问元数据
dependencies:
  - media_control/workspace
  - media_agents/runtime_session
related_workflow:
  - agent_session_startup
  - run_turn
  - agent_task_dispatch
  - platform_mcp_invocation
---

## 一句话职责

Agent Session Domain 拥有用户、Workspace 与 Agent 的稳定会话注册关系、Session Token 和 UI 元数据，不拥有 Runtime Turn、交互历史或事件序号。

## 边界

### 做什么

- 创建、查询、更新和删除 Agent Session Registration。
- 固定 Session 的 user、Workspace、Agent 和 context version。
- 在控制配置为空时，以 Default Agent Context 创建用户首个可对话 Session。
- 签发、Hash 保存并验证 Session Token，解析 Platform MCP 所需 Session Context。
- 在调用 Agent 命令和查询前执行 Session 所有权校验。
- 保存收藏、标题等 Gateway/UI 侧注册元数据。

### 不做什么

- 不保存 Turn、Queued Turn、Message、Universal Event、Agent Request 或 Chat Status；这些事实属于 `media_agents/runtime_session`。
- 不维护 pending message、Session Turn Queue 或 Session 执行锁。
- 不直连 Runtime SSE，不以 Gateway 进程、页面连接或节点亲和性决定 Turn 是否运行。
- 不直连 Agent MySQL；事件历史只能通过 Agent 查询接口读取。
- 不拥有 Workspace/Agent 期望配置。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理会话注册 | 管理 Agent Session Registration 生命周期和 UI 元数据 | `SessionService.Get/List/Update/Delete/Star` |
| 获取或创建默认会话 | 为已登录用户返回最近 Session；不存在时使用 Default Agent Context 创建 | `SessionService.GetOrCreateDefault` |
| 授权 Agent 调用 | 校验用户是否拥有目标 Session，再允许命令或查询转发 | `SessionService.Authorize`（目标接口） |
| 解析上下文 | 由 Session Token 返回固定身份上下文 | `ResolveSessionContext` |

Turn 提交、中断、Agent Request 响应和事件订阅是跨服务 Workflow，不是本 Domain 的状态变更能力。

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Agent Session Registration | Session Token Hash、UI Metadata | 注册、版本更新与删除 | user_id、workspace_id、agent_id 创建后固定；明文 Token 不落库 |

- `session_id` 同时作为 Agent Runtime Session 的外部关联 ID，但两侧禁止共享数据库表或建立跨服务外键。
- Gateway Registration active 不代表 Agent 当前没有 running Turn；Chat Status 必须查询 Agent 或读取其投影事件。
- 删除 Registration 必须通过启动/删除 Workflow 通知 Agent；仅删除 Gateway 行不能等价为终止 Runtime。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| `AgentSessionRegistered` | Session Registration 首次提交 | session_id, user_id, workspace_id, agent_id, context_version | Agent Session Startup |
| `AgentSessionDeleted` | Registration 进入 deleted | session_id, deleted_by | Agent Runtime、Analytics |

## 数据模型

### `agent_sessions`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Session ID；同时用于关联 Agent Runtime Session |
| user_id | text | 固定绑定的用户 ID |
| workspace_id | text | 固定绑定的 Workspace ID |
| agent_id | text | 固定绑定的 Agent ID |
| token_hash | bytea | Session Token 的 SHA-256 Hash |
| status | text | starting/active/failed/deleted |
| context_version | bigint | Session Context 版本 |
| title | text nullable | UI 展示标题 |
| starred | boolean | 用户是否收藏 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

#### 约束与索引

- Token Hash 唯一，明文 Token 只在受控启动链路交付。
- Session 所有读取和写入均按认证主体校验 `user_id`/Workspace 范围。
- 本表不保存 `active_turn_id`、`pending_message`、`chat_status` 或事件 sequence；迁移期旧列只允许兼容读取，禁止继续作为权威写入。

## 能力详细说明

### GetOrCreateDefault

1. 只接受认证上下文中的 `user_id`，不接受请求体指定所有者。
2. 优先返回该用户最近的 active Registration。
3. 若不存在，通过 media_control 解析 Workspace/Agent；本阶段可以使用内置 Codex Default Agent Context。
4. 在本地事务创建 starting Registration 和 Token Hash；事务提交后通过 `agent_session_startup` 注册 Agent Runtime Session。
5. Runtime 注册成功后 CAS 为 active；结果不明确时按 `session_id` 查询 Agent，禁止重新创建第二个 Session。

### Authorize

1. 从 Cookie JWT 或用户 Service Token 取得真实用户身份。
2. 加载 Registration，检查 status、user_id 和 Workspace 访问范围。
3. 生成只包含稳定 session/user/workspace/agent/context version 的内部命令；请求体同名字段不得覆盖。
4. Agent 仍校验调用方 mTLS 服务身份和 Runtime Session 注册关系。

### ResolveSessionContext

1. 从 mTLS 获取并校验调用服务 allowlist。
2. Hash 查询 Token，要求 Registration active 且未撤销。
3. 返回 user/workspace/session/agent、Registration status、context_version 和 expires_at，不返回任何 Token。

## 关联关系

### 依赖的 Domain

- `media_control/workspace`：提供 Workspace 访问和 Agent 运行上下文。
- `media_agents/runtime_session`：接受 Session 注册并拥有全部执行事实。

### 被依赖的 Domain

- `agent_task`：保存任务身份并引用目标 Session。
- `session_analytics`：使用 Registration 元数据关联 Agent Turn 事实。

### 参与的 Workflow

- [agent_session_startup](../../../workflow/0002-agent-session-startup/agent_session_startup.md)
- [run_turn](../../workflow/0001-run_turn/run_turn.md)
- [agent_task_dispatch](../../workflow/0002-agent_task_dispatch/agent_task_dispatch.md)

### 关联术语

- `Agent Session Registration`
- `Session Token`
- `Runtime Session`
