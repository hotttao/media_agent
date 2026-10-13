# Runtime Session 变更记录

## 2026-10-12 · DDD 收拢与核心接口模型验证

- 核心六能力与旧入口纳入覆盖矩阵，保持完整目标与 Plugin/Auth 外部所有者。
- 实例注册、执行所有权、配置发布/应用分开建模；迁移持久意图与 Agent epoch 双侧协调。
- queued 接管重取凭据，浏览器 Cookie 与稳定 Session Token 分离；原Session配置产物保持。
- 接口伪代码22场景通过，完整spec与plan已形成；模型验证不替代实际数据库/Broker/环境集成测试。

## 2026-10-11 · 缩容时 queued Turn 随 Session 重绑继续

- draining 副本完成当前 running Turn 后停止领取下一条，无需清空 queued 队列。
- 已接受 queued Turn 保留原 ID/FIFO/配置版本，由后台主动协调重绑，新副本获得所有权后继续，不等待新用户输入。
- 无可用候选时保持等待；原失败/中断暂停不隐式恢复，执行凭据在新副本按固定引用重新协调，不迁移秘密。

## 2026-10-11 · 参考 agent-platform 的副本路由与共享存储

- Session 绑定副本持久化；Ready 且非 draining 时沿用，扩容不改变有效绑定。
- 支持缩容及不可用/draining 副本上的 Session 重绑，替代本次讨论中较早的禁止缩容设想。
- 多副本共享 Workspace RWX 卷及 HOME；执行互斥在 Session 级，共享文件采用独立的原子发布规则。
- 保留本项目 Agent-owned FIFO；恢复扫描限定负责范围，重绑不得自动重复未知工具副作用。
- 运行所有权与排空准入的实现细节待 spec 明确，本次仅更新目标设计。

## 2026-10-11 · RabbitMQ 替换目标与执行实例术语

- RabbitMQ 完全替代 Redis；保留 delta 不落库、不占恢复序号的规则。
- 接受事实提交后投递的故障窗口，继续通过 Gateway 回放与补查恢复，不要求本次增加事件 Outbox。
- Session 继续固定配置版本，忽略研究文档中运行时更新提示词的设想。
- 区分稳定 Agent Identity 与可替换 Agent Runtime Instance；实例标识、代次与生命周期契约尚待细化。
- 本次记录目标设计，Redis 代码尚未替换；队列拓扑和元数据通知流程仍待对齐。

## 2026-08-22 · Agent Request 交互事实落地

- confirm、input 和 permission 请求与 `agent.requested` 事件在同一 PostgreSQL 事务持久化。
- 同一响应重试幂等，不同响应冲突；Turn 终态会取消 pending Request，晚到响应和 Runtime 事件不得改写历史。
- Agent API 提供请求查询和 resolve，Gateway 仅在授权后代理，不保存交互事实。

## 2026-08-22 · Universal Event、Message 历史与回放落地

- Runtime Session 新增 `next_message_sequence`，并发安全地分配历史 Message 查询次序。
- Message Sequence 与 Universal Event 的 Session Sequence 分别递增，禁止复用或以 `MAX+1` 推导。
- 现有 user Message 回填后从当前最大值继续，assistant/tool Message 使用同一计数器。
- 完成项、工具结果快照和终态事实由 Agent 原子持久化；Live Event Frame 不占 Session Sequence。
- 增加按 `after_sequence` 的 Universal Event 回放和稳定 Message 历史查询。

## 2026-08-22 · Execute Turn 终态与暂停规则落地

- running Turn 可原子进入 completed、failed 或 interrupted，终态不可覆盖。
- completed 在同一调度事务声明 FIFO 队首；failed/interrupted 把 Runtime Session 置为 paused 并保留 queued Turn。
- 增加 Turn 查询和软中断入口，Runtime Handle 随首次执行持久化。

## 2026-08-22 · 区分业务 Session 与 Runtime handle

- 新增可空 `runtime_session_id`，持久化 ACP/Codex 或 Claude Code 返回的底层会话句柄。
- 后续 Turn 与重启恢复使用该句柄；禁止假设它等于 Gateway 分配的业务 `session_id`。
- API key 等秘密通过 write-only 执行信封进入 workflow，只在对应 Turn 的内存执行上下文存在，不进入 Turn input、Message 或 Runtime Session 表；恢复时再按 `context_ref` 协调重取。

## 2026-08-22 · Submit Turn 持久化 FIFO 落地

- 使用 Session 行锁分配严格递增 enqueue sequence，并以 partial unique index 保证至多一个 running Turn。
- `session_id + idempotency_key` 重试返回既有 Turn，不重复写 user Message。
- Turn、user Message、Session 计数器和首条 Turn 声明在同一 PostgreSQL 事务内提交。

## 2026-08-22 · Agent 事实库统一为 PostgreSQL

- Runtime Session、Turn、Message、Universal Event 与 Agent Request 使用 Agent 自有 PostgreSQL database/schema。
- 数据库产品统一不改变 Agent 的事实所有权；Gateway 仍禁止直连并只能使用 Agent API。
- running Turn 唯一约束使用 PostgreSQL partial unique index 实现。

## 2026-08-18 · 接收 Gateway 的 Turn 与事件所有权

- Runtime Session 成为 Turn、Message、Universal Event、Session sequence、Chat Status 和 Agent Request 的唯一事实所有者。
- `pending_message` 单槽位改为持久化 Queued Turn；同一 Session 使用 FIFO 队列和 running 唯一约束串行执行。
- Universal Event 先提交 Agent 数据库，再发布 Session Event Bus；Gateway 通过 Agent 查询接口回放，不直连数据库。
- Gateway 节点、浏览器 SSE 连接和消息中间件 offset 均不参与 Turn 执行所有权判断。


### 2026-10-12 · P3 执行归属与 Session 迁移屏障

执行归属使用 Workspace/config/replica/incarnation 加 Session execution_epoch；托管写入及实时事件校验归属。单 Session 迁移使用持久 handoff_id，实例级 draining 留给缩容；原队列、上下文版本和 native handle 保留。Gateway 保存可恢复转移意图，查询实际归属收敛绑定，执行结果未知不重放 running。

## 2026-10-13 · P5 工作负载落地

- 按已验证 Gateway 登记结果初始化新代次 admission，支持退休序号重新运行，同时保留同代次及未知登记的排空屏障。
