# Agent Session 变更记录

## 2026-08-10 · 最小 Chat 默认上下文

- 增加 `GetOrCreateDefault`，让已登录但尚未配置 Workspace/Agent 的用户可以直接进入对话。
- media_control 空响应解释为“使用平台 Default Agent Context”，而不是启动失败。
- 明确 Universal Event 必须完整持久化，assistant/tool Message 由已提交的完成事件生成，不允许只保存 UI 摘要。

## 2026-08-11 · 多客户端会话事件订阅

- Universal Event sequence 统一为 Session 内严格递增游标。
- 增加按游标回放能力；进程内广播只负责低延迟分发，不替代持久化事实。
- 一个 Session 的多个页面订阅相互独立，任何客户端断开都不得取消 Turn。

## 2026-08-17 · 事件持久化分级与 Chat Status

- 把非持久化 Live Event Frame 与带 sequence 的 Universal Event 分开，恢复承诺收敛到最终 Message、工具快照和生命周期事实。
- tool result 改为首次立即写、300ms 聚合追加快照、终态强制 flush，并在持久化边界统一截断超大输出。
- Session 增加 `chat_status=human|agent` 交互控制投影；它不替代 Session 生命周期或 Turn 执行状态。

## 2026-08-17 · Turn 软中断

- 增加查询当前 running Turn 的领域能力，作为中断前的 Session 归属与执行状态事实。
- 用户中断不创建新的持久化实体，也不直接把 Turn 改为终态；Runtime 确认取消后，由既有执行流提交 `interrupted`。
- `interrupted` 与 `completed`、`failed` 同为不可互相覆盖的终态；中断前已提交的 Message、Universal Event 和 Tool Result Snapshot 全部保留。
