---
name: agent_session
description: Agent 持久化会话、Session Token 与交互记录
dependencies:
    - media_control/workspace
related_workflow:
    - agent_session_startup
    - run_turn
    - agent_task_dispatch
    - platform_mcp_invocation
---

## 一句话职责

Agent Session 负责用户在指定 Workspace 与 Agent 的持续交互边界、稳定 Session Token 及可恢复交互记录。

## 边界

### 做什么
- 创建、查询、更新、删除和中断 Agent Session。
- 保存 Turn、Message、Universal Event、Tool Result Snapshot 和 Agent Request；Live Event Frame 只属于运行期传输。
- 在控制配置为空时，以平台 Default Agent Context 创建用户的首个可对话 Session。
- 保证同一 Session 最多一个 active Turn，并保存一个待执行 follow-up。
- 校验 Session Token 并解析 Session Context。
- 按 Session 与 sequence 查询已提交 Universal Event，作为事件订阅的恢复事实源。

### 不做什么
- 不拥有 Workspace/Agent 期望配置。
- 不保存 Runtime 进程内状态，不代理业务 MCP。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理会话 | 管理 Agent Session 生命周期 | `SessionService.Get/List/Update/Delete/Star/Interrupt` |
| 获取或创建默认会话 | 为已登录用户返回最近 Session；不存在时使用 Default Agent Context 创建 | `SessionService.GetOrCreateDefault` |
| 运行交互 | 创建 Turn 并流式记录事件 | `ChatService.RunTurn/Reconnect/Respond` |
| 订阅会话事件 | 授权后按 sequence 回放 Universal Event 并接入实时广播 | `ChatService.SubscribeEvents` |
| 查询交互控制方 | 返回独立于 Session 生命周期的 human/agent Chat Status | `SessionService.Get/List` |
| 暂存 follow-up | Turn 运行期间原子保存下一条用户输入 | 待 IDL 定义 |
| 处理请求 | 查询和解决 Agent Request | `AgentRequestService.Get/Resolve` |
| 解析上下文 | 由 Session Token 返回固定身份上下文 | `ResolveSessionContext` |

## 数据模型

### `agent_sessions`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Session ID |
| user_id | text | 固定绑定的用户 ID |
| workspace_id | text | 固定绑定的 Workspace ID |
| agent_id | text | 固定绑定的 Agent ID |
| token_hash | bytea | Session Token 的 SHA-256 Hash |
| status | text | starting/active/interrupted/failed/deleted |
| chat_status | text | human/agent；当前交互控制方的可查询投影 |
| context_version | bigint | Session Context 版本 |
| active_turn_id | text nullable | 当前 pending/running Turn 的逻辑引用 |
| pending_message | jsonb nullable | 运行期间提交的唯一待执行 follow-up |
| pending_message_version | bigint | pending_message 的 CAS 版本 |
| starred | boolean | 用户是否收藏 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `turns`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Turn ID |
| session_id | text | 所属 Agent Session |
| sequence | bigint | Session 内轮次序号 |
| status | text | pending/running/completed/failed/interrupted |
| started_at | timestamptz nullable | 开始时间 |
| finished_at | timestamptz nullable | 结束时间 |

#### 约束与索引

- `UNIQUE (session_id) WHERE status IN ('pending', 'running')`，数据库层禁止并发 Turn。
- `active_turn_id` 必须引用本 Session 的 pending/running Turn；终态提交时原子清空。
- pending message 每个 Session 只有一份；新 follow-up 按产品合并/替换规则更新同一记录，不创建并发 Turn。

### `messages`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Message ID |
| session_id | text | 所属 Agent Session |
| turn_id | text | 所属 Turn |
| role | text | user/assistant/tool/system |
| content_ref | jsonb | 文本内容或外部内容引用 |
| sequence | bigint | Session 内消息序号 |
| created_at | timestamptz | 创建时间 |

### `universal_events`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Universal Event ID |
| session_id | text | 所属 Agent Session |
| turn_id | text nullable | 关联 Turn |
| sequence | bigint | Session 内严格递增事件游标 |
| type | text | 事件类型 |
| payload | jsonb | 事件载荷 |
| created_at | timestamptz | 产生时间 |

Universal Event 只保存可恢复的权威事实。`item.started`、`item.delta` 等 Live Event Frame 不写入本表；tool result 每次聚合 flush 以新的 sequence 追加 Tool Result Snapshot，不覆盖旧 sequence。

### `agent_requests`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Request ID |
| session_id | text | 所属 Agent Session |
| turn_id | text | 所属 Turn |
| type | text | confirm/input/permission 等请求类型 |
| payload | jsonb | 请求内容 |
| status | text | pending/resolved/cancelled/expired |
| response | jsonb nullable | 用户响应 |
| resolved_at | timestamptz nullable | 解决时间 |
| created_at | timestamptz | 创建时间 |

### ID 生成规则

当前最小 Chat 的业务 ID 使用 UUID v4；后续 Session Token 使用至少 256 位随机数，数据库只保存 Hash，明文仅在受控启动链路交付。

## 能力详细说明

### RunTurn / 暂存 follow-up

1. Session 没有 active Turn 时，在一个事务中创建 pending Turn、User Message，并设置 `active_turn_id`。
2. Session 已有 pending/running Turn 时，不创建新 Turn；原子写入或合并 `pending_message`，返回 queued。
3. 当前 Turn completed 后，原子 take-and-clear pending message，再创建下一 Turn；派发失败时仅在没有更新版本时恢复旧 pending message。
4. 当前 Turn failed/interrupted 时不自动消费 pending message，保留给用户继续、修改或清除。
5. 客户端连接断开不改变 active Turn，也不触发 pending message 消费。

### Session Event Subscription

1. 订阅前必须校验 Session 归属，客户端只能订阅自己拥有的 Session。
2. `after_sequence` 表示客户端已处理的最后一个事件；服务端只交付更大的 sequence。
3. 订阅先注册实时接收者，再查询持久化事件；实时阶段丢弃不大于已交付游标的重复事件，因此回放与实时切换无缺口。
4. 一个 Session 可以同时存在多个订阅，每个订阅独立断开，不能影响其他订阅或 active Turn。
5. 广播是进程内投递优化，Universal Event 才是恢复事实；断线、慢消费者或 Gateway 重启均通过 sequence 回放恢复。
6. 无 SSE id 的 Live Event Frame 可以实时更新 UI，但不得推进客户端 sequence cursor；刷新后由 Message、Tool Result Snapshot 和 Session 终态恢复。

### Chat Status

1. `status` 表示 Session 生命周期是否可用；`chat_status` 只表示当前交互由 human 还是 agent 掌控，两者禁止合并。
2. 创建 running Turn 的同一事务把 `chat_status` 从 human 切换为 agent。
3. Runtime 的所有待写完成项和 Tool Result Snapshot flush 后，Turn 终态与 `chat_status=human` 在同一事务提交。
4. Turn 是执行状态的权威来源；若投影异常，恢复任务必须根据是否存在 running Turn 修正 `chat_status`。

### Runtime Event Durability

1. 文本 delta 只作为 Live Event Frame 广播；message kind 的 `item.completed` 整块生成 assistant Message 和 Universal Event。
2. tool call 在完成时以稳定的 item/call 标识幂等持久化一次。
3. tool result 首次立即形成快照，后续更新按 300ms 聚合；每次 flush 追加新的 sequence，终态 flush 保证最终快照先于 Turn 终态提交。
4. tool result 在统一规范化边界按 Runtime capability 做一次 UTF-8 安全截断，清除 NUL，并记录是否截断、原始字节数和持久化字节数。

### GetOrCreateDefault

1. 只接受认证上下文中的 `user_id`，不接受请求体指定所有者。
2. 优先返回该用户最近的 active Session。
3. 若不存在，通过 media_control 配置接口解析 Workspace/Agent；本阶段由内置 mock provider 返回合理的 Codex Default Agent Context。
4. 在一个事务中创建 active Session；并发首次访问按用户与默认上下文幂等收敛到同一 Session。

### ResolveSessionContext

1. 从 mTLS 获取并校验调用服务 allowlist。
2. Hash 查询 Token，要求 Session active 且未撤销。
3. 返回 user/workspace/session/agent、状态、context_version 和 expires_at，不返回任何 Token。
