# 区分持久化 Agent 事件与临时流式帧

Agent Runtime 的高频 delta 和持续增长的 tool result 不再逐帧写入数据库：Gateway 将其区分为不推进恢复游标的 Live Event Frame，以及带 Session sequence、可回放的 Universal Event。文本以 `item.completed` 为权威事实；tool call 完成时单次持久化；tool result 首次立即持久化、之后按聚合窗口追加最新快照，并在 Turn 终止前强制 flush。选择追加新 sequence 而不是覆盖旧事件，是为了保留现有 `after_sequence` 回放和去重语义；选择最终事实而非完整传输帧日志，是为了控制数据库写放大和超大工具输出。

工具输出在进入持久化与回放边界时按 Runtime capability 做一次 UTF-8 安全截断，并记录截断元数据。在线 UI 可以消费无 SSE id 的临时帧，但只有带 id 的持久化事件才能推进客户端 cursor；因此断线恢复保证最终 Message、工具最新快照和 Session 终态不丢，不保证重放每一个流式 delta。

Session 另外保存 `chat_status = human | agent` 作为当前交互控制方的可查询投影，避免刷新后的 UI 依赖偶发的 live frame 判断后台 Turn 是否仍在运行。它不与 Session 生命周期 `status` 合并，也不替代 Turn 的权威执行状态；创建 Turn 时切到 agent，所有待写事实 flush 并提交 Turn 终态后切回 human。
