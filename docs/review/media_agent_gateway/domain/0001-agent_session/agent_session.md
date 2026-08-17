# Agent Session SSE 设计对齐审查

## 审查结论

当前 `media_agent_gateway` 的 SSE 模型不是 `agent-platform/control-plane` 模型的等价迁移。差异在设计阶段已经形成，随后实现基本按本项目的新设计落地；因此主因不是“设计对齐但实现写偏”，而是“设计先主动分叉，局部实现再存在少量未完全落地项”。

时间证据：

- `53abd4b`（2026-08-11 15:42:14）先修改 Agent Session / Run Turn 设计并新增 Session Event Broadcast spec；
- `c67c9d3`（2026-08-11 15:43:19）随后实现 Gateway replayable events；
- `7c44302`（2026-08-11 15:43:32）随后实现 UI session chat。

## 领域语义分叉

| 语义 | agent-platform | 本项目目标设计 | 判断 |
|:---|:---|:---|:---|
| 恢复事实源 | 完成态 Message；活动流由 CP/Agent 内存承载 | PostgreSQL `Universal Event` | 设计分叉 |
| `item.delta` | 不持久化，`item.completed` 携带权威完整文本 | 所有 Runtime Universal Event 完整持久化 | 设计分叉 |
| 重连 | `cp-reconnect` 仅附着到活动流，不回放历史 | `SubscribeEvents(after_sequence)` 先回放再接 live | 设计分叉 |
| 游标 | reconnect 接口无持久化 sequence 游标 | Session 内严格递增 sequence | 设计分叉 |
| 连接与 Turn | 浏览器断开不终止 Turn | 浏览器断开不终止 Turn | 对齐 |
| 多客户端 | 一个 ActiveSSEStream 向多个 controller fan-out | SessionBroadcaster 向多个 subscriber fan-out | 能力对齐、机制不同 |

agent-platform 的 `createReconnectSSEResponse` 明确声明调用方应先加载 DB Message，reconnect 只接收后续事件；其持久化插件明确跳过 `item.delta`。本项目 `CONTEXT.md` 和 Agent Session Domain 则把 Universal Event 定义为“可恢复、可排序的运行事件记录”，并把它指定为事件订阅的恢复事实源。两者不是同一通用语言。

## 本项目设计与实现映射

| 能力 | 设计证据 | 实现证据 | 状态 |
|:---|:---|:---|:---|
| Universal Event 全量落库 | Agent Session `change.md`、Run Turn Workflow | `Service.PersistEvent` 对每个 Runtime JSON event 写 `universal_events` | covered |
| `item.completed` 同事务生成 Message | Agent Session Domain / Chat spec | `PersistEvent` 构造 Message 后调用 `AppendEvent` | covered |
| sequence 查询回放 | Session Event Subscription | `ListEventsAfter(sequence)` 按 sequence 升序查询 | covered |
| 多订阅隔离 | Session Event Subscription | `Broadcaster` 为每个 subscriber 建独立 channel | covered |
| 慢订阅关闭且不影响其他订阅 | Session Event Broadcast spec | `Publish` 在 channel 满时只关闭该 subscriber | covered |
| 先授权再订阅 | Agent Session Subscription 规则 1 | Handler 先 `Subscribe`，再通过 `EventsAfter/GetOwned` 校验归属 | partial |
| Session sequence 严格递增 | `universal_events.sequence` 不变量 | `MAX(sequence)+1`，没有显式 Session 行锁或原子计数器 | partial |

### 局部实现偏差

1. **授权顺序偏差（低风险）**：设计要求订阅前校验 Session 归属；实现先注册内存 subscriber，再查询归属。未授权事件不会写入 HTTP 响应，且退出时会关闭 subscriber，但执行顺序不符合领域规则。
2. **sequence 并发保证不足（中风险）**：Domain 声明 Session 内严格递增，Workflow 还要求重复/缺口检测；当前以 `SELECT MAX(sequence)+1` 分配。单一串行 Runtime callback 下通常成立，但数据库层没有证明并发写入时仍满足不变量。

## 能力覆盖矩阵

| 原能力 | 目标上下文 | Domain / Workflow | 状态 |
|:---|:---|:---|:---|
| agent-platform live-only reconnect | media_agent_gateway | 被 replayable Session Event Subscription 替代 | deprecated-with-reason：本项目选择持久化事件恢复 |
| 多客户端实时接收 | media_agent_gateway | agent_session / run_turn | covered |
| 页面刷新后恢复最终消息 | media_agent_gateway | Message 历史 + Universal Event replay | covered |
| 页面刷新后恢复流式 delta | media_agent_gateway | Universal Event replay | covered（能力强于参考实现） |
| Gateway 重启后恢复浏览器事件 | media_agent_gateway | 已提交 Universal Event replay | covered |
| Gateway 重启后恢复正在运行的 Runtime Turn | media_agent_gateway | 当前 spec 明确排除 | missing：Agent Runtime 尚有流状态，但 Gateway 没有 agent-platform 的 orphan recovery workflow |

## 结论边界

如果验收标准是“最终消息在刷新后完整”，两种模型都能达到；如果验收标准是“接口、恢复事实和重连语义与 agent-platform 相同”，本项目从 2026-08-11 的设计提交起就已不对齐。是否回归 agent-platform 模型属于目标设计决策，不能作为单纯实现修复处理。由于 Runtime 跨 Gateway 重启恢复仍为 `missing`，本次能力覆盖审查只能判定为部分完成。
