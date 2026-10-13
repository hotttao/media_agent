# Run Turn 变更记录

## 2026-10-11 · Gateway 实例级临时队列与服务内广播

- 每个 Gateway 实例对应一个会话事件临时队列，消费后由服务内 Broadcaster 按 Session 扇出到本机 SSE。
- 每个 SSE 只注册本机接收者并持有独立恢复 cursor，不创建 RabbitMQ 队列。
- 实例队列与连接生命周期解耦；重建队列后以 Agent 回放恢复持久事件，delta 仍不补发。

## 2026-10-11 · RabbitMQ 替换目标与执行实例术语

- RabbitMQ 完全替代 Redis；保留 delta 不落库、不占恢复序号的规则。
- 接受事实提交后投递的故障窗口，继续通过 Gateway 回放与补查恢复，不要求本次增加事件 Outbox。
- Session 继续固定配置版本，忽略研究文档中运行时更新提示词的设想。
- 区分稳定 Agent Identity 与可替换 Agent Runtime Instance；实例标识、代次与生命周期契约尚待细化。
- 本次记录目标设计，Redis 代码尚未替换；队列拓扑和元数据通知流程仍待对齐。

## 2026-08-22 · Agent 事实库统一为 PostgreSQL

- Gateway 与 Agent 的命令、回放和补查契约不变，仅把 Agent 权威库从 MySQL 修订为 PostgreSQL。

## 2026-08-18 · Gateway 改为 Agent 命令与事件适配层

- Turn 接受、持久化排队、串行执行、Message/Universal Event 写入和 Session sequence 全部迁入 Agent。
- Gateway 不再直连 Runtime SSE 或维护进程内事实广播；改为订阅 Session Event Bus，并通过 Agent Replay API 回放。
- 订阅固定采用“先订阅并缓存，再回放，再按 sequence 合并去重”；跳号、周期和终态触发补查。
- Agent 数据库提交后 Redis 发布失败不回滚事实，当前连接和页面重连均通过持久化补查恢复。
- pending message 单槽位被 Agent Queued Turn FIFO 取代；同 Session 串行性不再依赖 Gateway 节点。

## 2026-08-17 · 采用 Runtime 软中断

- 用户中断只停止当前 running Turn，不终止 Agent Session，也不回滚已经发生的外部副作用。
- Gateway 不持久化中断请求、不增加中间状态和后台重试；Runtime 不可达或未找到 active Turn 时直接返回失败。
- Runtime 使用运行期取消能力停止输出并发送 `TurnEnded(reason=interrupted)`；Gateway 保留已接收事件并复用 TX-FINAL 提交 Turn 终态。
- 删除 `interrupt_requested` 和中断请求元数据，Turn 状态保持 pending/running/completed/failed/interrupted。

## 2026-08-10 · 最小 Chat 闭环

- 首次对话支持通过 Default Agent Context 创建 Session。
- 明确 Web 用户身份由 media_auth Cookie Session 解析，空控制配置不绕过认证。
- 强化事件顺序：每个 Runtime Universal Event 先完整落库再推送，`item.completed` 同事务生成完整 Message。

## 2026-08-11 · Turn 生命周期与客户端订阅解耦

- `RunTurn` 改为提交 Turn 后返回 accepted，由后台执行 Runtime，不再把发起请求作为唯一 SSE 下游。
- 持久化后的事件广播给同一 Session 的全部页面；订阅按 sequence 回放并去重。
- 明确发起页面、其他页面和断线重连都使用相同 Session Event Subscription。

## 2026-08-17 · 按事件 kind 控制持久化写放大

- 将 Runtime 输出区分为非持久化 Live Event Frame 与可回放 Universal Event；只有后者分配 sequence 并推进客户端 cursor。
- 文本在 `item.completed` 整块落盘，tool call 完成时幂等写入，tool result 首次立即写入并按 300ms 聚合追加快照。
- 所有终止路径必须先强制 flush pending tool result，再提交并广播 Turn 终态。
- tool result 在统一持久化边界按 Runtime capability 做一次 UTF-8 安全截断，并保存截断元数据。
- 新增独立于 Session 生命周期的 `chat_status=human|agent`：创建 Turn 时切到 agent，终态 flush 完成后切回 human，供刷新后的客户端查询后台执行状态。


### 2026-10-12 · P3 执行归属与 Session 迁移屏障

执行归属使用 Workspace/config/replica/incarnation 加 Session execution_epoch；托管写入及实时事件校验归属。单 Session 迁移使用持久 handoff_id，实例级 draining 留给缩容；原队列、上下文版本和 native handle 保留。Gateway 保存可恢复转移意图，查询实际归属收敛绑定，执行结果未知不重放 running。
