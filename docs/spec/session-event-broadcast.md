# Session Event Broadcast 实现规格

## 范围

跨 `media_agent_gateway` 与 `ui_platform` 实现一个 Session 多页面实时广播。Turn 执行生命周期与浏览器请求解耦；PostgreSQL Universal Event 是恢复事实，进程内 broadcaster 是低延迟分发层。

## HTTP 契约

- `POST /api/chat/sessions/:session_id/turns`：请求体保持 `{message,idempotency_key}`，成功返回 `202 {turn_id,status:"accepted"}`。
- `GET /api/chat/sessions/:session_id/events?after_sequence=N`：返回 `text/event-stream`。每个事件使用 `id: <sequence>`、`event: message`、`data: <UniversalEvent JSON>`；空闲时发送注释 heartbeat。
- 鉴权沿用 Cookie JWT；两个入口都校验 Session 所有者。

## Gateway

- `SessionBroadcaster` 以 `session_id` 分区，保存独立 subscriber channel；慢订阅者缓冲溢出时仅关闭该订阅。
- Subscribe 顺序固定为：注册 subscriber → 查询 `sequence > after_sequence` → 写回放 → 消费 live；用当前 cursor 丢弃重叠事件。
- Runtime 在后台 Context 中运行。每个事件在事务中获得 Session 内 sequence 并持久化，提交成功后才 Publish。
- Gateway 重启会丢失 live subscriber 和后台执行句柄，但已提交事件仍可回放；Runtime 跨 Gateway 重启恢复不属于本次范围。

## UI

- Chat App 在选定 Session 后建立 events fetch 流，并在 cleanup 中 abort。
- `POST /turns` 只处理 accepted/error；实时增量只从订阅进入，避免发起页面重复消费。
- 每个 Session 维护内存 cursor；重连携带 `after_sequence`，事件按 sequence 去重。
- `session.ended` 或 `error` 后刷新 Message 历史；页面切换隔离旧订阅。

## 验证

- Go：广播 fan-out、取消隔离、慢订阅、回放/live 重叠去重、后台请求断开不取消 Turn。
- media-cli：一个 Turn 发起方加两个 SSE 订阅者收到同序列事件。
- Playwright：两个页面打开同一 Session，一页发送，两个页面均显示 assistant 回复。
