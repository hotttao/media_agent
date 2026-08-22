# Agent 持有 Turn 调度、交互事实与事件发布

状态：accepted；其中数据库产品选择已由 [ADR-0010](0010-agent-runtime-postgresql.md) 修订，领域所有权与事件发布决策继续有效。

Agent 是 Runtime Session、Turn 队列、Message、Universal Event、Session sequence 和 Chat Status 的唯一权威写入方；Gateway 只保留 Agent Session Registration、Web 鉴权、命令/查询转发及 SSE framing。Agent 先把 Universal Event 提交到自己的 MySQL，再发布到 Session Event Bus；Gateway 建立订阅时先注册实时接收者，再通过 Agent 查询接口回放 `sequence > after_sequence` 的持久化事件，并按 sequence 去重。初始总线采用 Redis Pub/Sub 的按 Session 广播语义，因为 MySQL 已承担恢复事实，Gateway 还会在序号缺口、终态和周期检查时补查持久化事件。

该边界取代“Gateway 直连 Runtime SSE、持久化交互并使用进程内 broadcaster”的设计。选择下沉到 Agent，是因为只有 Agent 能最终约束同一 Runtime Session 的执行：提交输入先形成持久化 Queued Turn，Session Turn Queue 按 enqueue sequence 串行声明 running Turn，进程内 Session Lock 仅优化同进程竞争，数据库中“每个 Session 最多一个 running Turn”的约束负责跨线程、跨实例和重启后的最终正确性。Gateway 节点、浏览器 SSE 连接和负载均衡亲和性都不参与 Turn 所有权判断。

没有选择继续让 Gateway 持有事件，是因为多个 Gateway 的 Runtime SSE 与页面 SSE 可能落在不同节点；没有为通知通道采用 Kafka Consumer Group，是因为组内分摊不能天然把同一事件交付给所有持有相关 SSE 的 Gateway。Kafka、NATS JetStream 或 Redis Streams 仍可替换 Redis Pub/Sub，但必须保持按 Session 广播、业务 sequence 不变和 Agent MySQL 回放契约。

本决策不要求 MySQL 与消息总线组成分布式事务。数据库提交成功而发布失败时，事件仍是已提交事实；当前连接通过 sequence 缺口检测、周期补查或终态补查最终收到，页面重连则必然通过持久化回放恢复。若未来要求故障窗口内也必须即时通知，再单独引入 Outbox，不改变本 ADR 的领域所有权。
