# Execute Turn 变更记录

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

## 2026-08-22 · Agent Request 与重启恢复落地

- Codex ACP permission 请求先持久化再等待用户响应，resolve 提交后唤醒原 Runtime 调用。
- 进程启动扫描 running Turn；当 adapter 无法证明旧执行结果或重新挂接时，以 `recovery_outcome_unknown` 失败并暂停队列，禁止重放可能已产生副作用的 Turn。
- 多副本同时恢复容忍其他副本已提交的终态，不重复发布终态事件。

## 2026-08-22 · Runtime 事件持久化与提交后发布落地

- TX-ENQUEUE 和 TX-EVENT 均在 Session 行锁内分配 Message Sequence。
- Universal Event 仍只推进 Session Sequence；Message 查询顺序不参与事件回放去重。
- Codex ACP 与 Claude Code 的运行事件统一进入 Turn Event Sink；Live Frame 直接发布，权威事件提交 PostgreSQL 后发布。
- Tool Result 首次快照立即提交，后续按 300ms 合并最新值，终态前强制 flush 并执行 UTF-8 安全截断。
- Redis Pub/Sub 只承担多订阅者瞬时扇出；发布失败不回滚事实，调用方通过 Agent 回放接口恢复。

## 2026-08-22 · Codex ACP 与 Claude Code 执行编排落地

- 两种 Runtime 通过统一 port 接入 Execute Turn Flow，所有外部调用均位于数据库事务外。
- workflow 在后台串行排空 completed 队列；Runtime 失败和确认中断提交稳定终态并暂停队列。
- SubmitTurn write-only 信封支持 API key/Session Token，秘密不进入 input、响应、日志或 PostgreSQL，失败/中断后清除内存副本。

## 2026-08-22 · 增加 Runtime handle 建立步骤

- 首个 Turn 创建底层 Runtime 会话并持久化返回句柄；后续 Turn 加载该句柄。
- Runtime 调用保持在数据库事务外，只有句柄和终态属于 Agent 事务事实。
- 执行凭据通过 write-only 信封传入并仅在 Turn 内存上下文暂存，禁止随持久化 input 传入；重启恢复按版本化 `context_ref` 重新协调。

## 2026-08-22 · TX-ENQUEUE 与首次 TX-CLAIM 落地

- Agent HTTP API 接受幂等 SubmitTurn，并原子持久化 queued Turn 与 user Message。
- 无 active Turn 时在同一调度临界区声明队首 running；已有 active Turn 时保持 FIFO queued。
- 并发、重复提交、事务回滚与真实 HTTP media-cli E2E 已覆盖；Runtime 执行与终态留在下一切片。

## 2026-08-22 · 执行事实库统一为 PostgreSQL

- TX-ENQUEUE、TX-CLAIM、TX-EVENT、TX-SNAPSHOT 与 TX-FINAL 均提交到 Agent 自有 PostgreSQL。
- 事务、事件发布与 Gateway 补查语义不变；只修订数据库产品选择。

## 2026-08-18 · 从 Gateway 接管完整执行闭环

- Agent 先持久化 Queued Turn，再按 Runtime Session FIFO 串行声明 running Turn。
- Agent 负责 Runtime 输出分级、Message/Universal Event 持久化、Session sequence 和终态 flush。
- 事件提交后发布 Session Event Bus；发布失败由 Gateway 持久化回放和补查恢复，不回滚执行事实。
- failed/interrupted 后暂停自动排空；自然 completed 才自动拉起下一 queued Turn。


### 2026-10-12 · P3 执行归属与 Session 迁移屏障

执行归属使用 Workspace/config/replica/incarnation 加 Session execution_epoch；托管写入及实时事件校验归属。单 Session 迁移使用持久 handoff_id，实例级 draining 留给缩容；原队列、上下文版本和 native handle 保留。Gateway 保存可恢复转移意图，查询实际归属收敛绑定，执行结果未知不重放 running。

## 2026-10-13 · P6 ACP 边界与类型验收

ACP 会话仅依赖 prompt/cancel/is_alive 结构契约；Clock 与 draft ID 纯辅助能力归入 ACP 自有包，消除 Domain 对外层 shared 的反向依赖。流队列通过内部 typed Queue 提供只读快照，维持唯一 FIFO 与重连顺序。SDK 回调、MCP server union 和测试替身签名按现有 SDK 契约补齐，不调整执行规则。

## 2026-10-16 · 原生账号扩展

真实 OAuth 模型验收发现 Codex 默认连接账号 Apps/MCP 并同步远端 Plugin。固定元数据原生配置显式关闭这些自动装配及 Skill MCP 依赖安装，确保扩展事实仍来自 Control/Gateway，保留本地固定 Skill 与模型认证。

真实第二轮模型验收发现 ACP load_session 历史片段被拼入新消息。每轮 prompt 前重建翻译器，隔离加载历史与新输出；回归覆盖旧消息/工具状态不进入新完成项。

真实 MCP 配置验收补齐 Agent 对 media_plugin.mcp_binding 固定引用的 schema 支持；沿用 ID/版本/校验和校验，不把上游地址或凭据写入原生配置。
