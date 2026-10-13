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
  - session_replica_routing
---

## 一句话职责

Agent Session Domain 拥有用户、Workspace 与 Agent 的稳定会话注册关系、Session Token 和 UI 元数据，不拥有 Runtime Turn、交互历史或事件序号。

## 边界

### 做什么

- 创建、查询、更新和删除 Agent Session Registration。
- 固定 Session 的 user、Workspace、Agent 和 context version。
- 保存可变的 Session Replica Binding；副本切换不改变 Session 身份或配置版本。
- 在控制配置为空时，以 Default Agent Context 创建用户首个可对话 Session。
- 签发、Hash 保存并验证 Session Token，解析 Platform MCP 所需 Session Context。
- 在调用 Agent 命令和查询前执行 Session 所有权校验。
- 保存收藏、标题等 Gateway/UI 侧注册元数据。

### 不做什么

- 不保存 Turn、Queued Turn、Message、Universal Event、Agent Request 或 Chat Status；这些事实属于 `media_agents/runtime_session`。
- 不维护 pending message、Session Turn Queue 或 Session 执行锁。
- 不直连 Runtime SSE，不以 Gateway 进程、页面连接或节点亲和性决定 Turn 是否运行。
- 不直连 Agent PostgreSQL；事件历史只能通过 Agent 查询接口读取。
- 不拥有 Workspace/Agent 期望配置。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理会话注册 | 管理 Agent Session Registration 生命周期和 UI 元数据 | `SessionService.Get/List/Update/Delete/Star` |
| 获取或创建默认会话 | 为已登录用户返回最近 Session；不存在时使用 Default Agent Context 创建 | `SessionService.GetOrCreateDefault` |
| 授权 Agent 调用 | 校验用户是否拥有目标 Session，再允许命令或查询转发 | `SessionService.Authorize`（目标接口） |
| 解析上下文 | 由 Session Token 返回固定身份上下文 | `ResolveSessionContext` |
| 判断配置兼容性 | 向客户端公开 Session Context Version；无法取得同一固定上下文时拒绝新 Turn | `SessionResponse.context_version`、`RunTurn` |

Turn 提交、中断、Agent Request 响应和事件订阅是跨服务 Workflow，不是本 Domain 的状态变更能力。

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Agent Session Registration | Session Token Hash、Replica Binding/迁移意图、UI Metadata | 注册、版本更新与删除 | user_id、workspace_id、agent_id 创建后固定；明文 Token 不落库 |

- `session_id` 同时作为 Agent Runtime Session 的外部关联 ID，但两侧禁止共享数据库表或建立跨服务外键。
- `agent_id`、`agent_type` 与 `context_version` 在 Registration 创建后不可变；Workspace 配置升级不会迁移或覆盖既有 Registration。
- Session 使用创建时固定上下文；用户确认更新只改变后续新 Session 的默认配置。固定历史上下文无法取得时拒绝新 Turn，仍可读取历史并中断原 running。
- 同一 Session 的绑定变更以 binding_version 并发校验；重复请求必须使用数据库已提交的绑定，不以各 Gateway 的内存选取结果分别调用不同副本。
- 绑定副本 Ready 且未 draining 时继续沿用；不可用或 draining 时按 session_replica_routing 协调重绑。
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
| token_hash | text | Session Token 的 SHA-256 Hash |
| status | text | starting/active/failed/deleted |
| context_version | bigint | Session Context 版本 |
| environment_id | text | Control 已授权环境，来自原生资源登记 |
| incarnation_id | text | 当前路由所绑定的副本启动代次 |
| execution_epoch | bigint | 已确认 Agent 执行代次；不能替代 Agent 原子校验 |
| replica_id | text nullable | 所属 Workspace 内选定副本的稳定引用，Kubernetes 对应 StatefulSet ordinal |
| binding_version | bigint | 副本绑定的并发更新版本 |
| binding_phase | text | unbound/active/transferring；转移期间阻止新路由执行，源 running 可按 epoch 完成 |
| transfer_id | text nullable | 固定迁移操作ID，结果不明时查询恢复 |
| target_replica_id / target_incarnation_id | text nullable | 当前迁移意图的受控目标 |
| expected_execution_epoch | bigint | 当前迁移所依据的Agent执行代次 |
| token_key_id | text | 稳定会话令牌生成所用密钥版本 |
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

1. 从 Web Session 或用户 API Token 形成的 Internal JWT 取得真实用户身份。
2. 加载 Registration，检查 status、user_id 和 Workspace 访问范围。
3. 生成只包含稳定 session/user/workspace/agent/context version 的内部命令；请求体同名字段不得覆盖。
4. 托管 Agent 校验 Gateway Ed25519 请求签名、Workspace/config/实例代次及 Session execution_epoch。

### Configuration Compatibility

1. `SessionResponse` 返回固定的 `context_version` 与 `agent_type`，客户端可与 Workspace 当前配置版本比较。
2. 新 Turn 提交前，Gateway 从 Control 读取 Session 固定版本的历史 Runtime Context 并重新授权；若实际返回的 agent 或版本与 Registration 不一致，返回 HTTP 409 和稳定的 `ErrContextChanged` 领域错误，不得调用 Agent。
3. 历史 Message/Event 查询按 Registration 固定的 Agent Type 路由，不依赖 Workspace 当前配置。
4. Interrupt 不需要 Provider API Key，必须按 Registration 固定的 Agent Type 路由，使配置切换前已运行的 Turn 仍可停止。

### ResolveSessionContext

1. 从 mTLS 获取并校验调用服务 allowlist。
2. Hash 查询 Token，要求 Registration active 且未撤销。
3. 返回 user/workspace/session/agent、Registration status、context_version 和 expires_at，不返回任何 Token。

### Session Replica Binding

Session 注册时选定副本并持久化绑定。SubmitTurn 使用已确认绑定；Interrupt/Agent Request 响应查询实际 owner，历史与回放允许同一 Workspace/config 的其他已核验实例提供共享事实。副本切换只修改路由事实，不重新创建 Session、Token 或 Runtime Handle；切换必须先协调旧 Turn 的结果，不能把 Ready 状态变化直接当成执行已停止。

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
