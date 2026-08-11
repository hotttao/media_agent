# Agent Session 变更记录

## 2026-08-10 · 最小 Chat 默认上下文

- 增加 `GetOrCreateDefault`，让已登录但尚未配置 Workspace/Agent 的用户可以直接进入对话。
- media_control 空响应解释为“使用平台 Default Agent Context”，而不是启动失败。
- 明确 Universal Event 必须完整持久化，assistant/tool Message 由已提交的完成事件生成，不允许只保存 UI 摘要。

## 2026-08-11 · 多客户端会话事件订阅

- Universal Event sequence 统一为 Session 内严格递增游标。
- 增加按游标回放能力；进程内广播只负责低延迟分发，不替代持久化事实。
- 一个 Session 的多个页面订阅相互独立，任何客户端断开都不得取消 Turn。
