# Run Turn 变更记录

## 2026-08-10 · 最小 Chat 闭环

- 首次对话支持通过 Default Agent Context 创建 Session。
- 明确 Web 用户身份由 media_auth Cookie Session 解析，空控制配置不绕过认证。
- 强化事件顺序：每个 Runtime Universal Event 先完整落库再推送，`item.completed` 同事务生成完整 Message。

## 2026-08-11 · Turn 生命周期与客户端订阅解耦

- `RunTurn` 改为提交 Turn 后返回 accepted，由后台执行 Runtime，不再把发起请求作为唯一 SSE 下游。
- 持久化后的事件广播给同一 Session 的全部页面；订阅按 sequence 回放并去重。
- 明确发起页面、其他页面和断线重连都使用相同 Session Event Subscription。
