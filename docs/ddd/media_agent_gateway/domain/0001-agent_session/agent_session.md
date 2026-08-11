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
- 保存 Turn、Message、Universal Event 和 Agent Request。
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

### GetOrCreateDefault

1. 只接受认证上下文中的 `user_id`，不接受请求体指定所有者。
2. 优先返回该用户最近的 active Session。
3. 若不存在，通过 media_control 配置接口解析 Workspace/Agent；本阶段由内置 mock provider 返回合理的 Codex Default Agent Context。
4. 在一个事务中创建 active Session；并发首次访问按用户与默认上下文幂等收敛到同一 Session。

### ResolveSessionContext

1. 从 mTLS 获取并校验调用服务 allowlist。
2. Hash 查询 Token，要求 Session active 且未撤销。
3. 返回 user/workspace/session/agent、状态、context_version 和 expires_at，不返回任何 Token。
