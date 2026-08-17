# Run Turn 变更记录

## 2026-08-17 · 中断改为请求与确认两阶段

- running Turn 先持久化为 `interrupt_requested`，再在事务外向 Runtime 发送幂等取消命令。
- 只有 Runtime 的权威终止结果可以提交 `interrupted`；Runtime 不可达时保持请求态并重试/协调。
- pending Turn 尚未启动 Runtime 时允许本地直接中断；自然完成或失败一旦提交，晚到中断不得覆盖终态。
- 区分取消请求是否送达与 Turn 是否真正中断，供 Web UI 准确展示停止结果。

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
