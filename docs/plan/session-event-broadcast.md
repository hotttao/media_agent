# Session Event Broadcast 落地计划

## 文档

- [x] 更新 Agent Session、Run Turn、UI 状态与 Chat 旅程。
- [x] 定义跨 Gateway/UI 实现规格。

## Gateway

- [x] 增加 SessionBroadcaster 与并发测试。
- [x] 增加 Session 内 Event sequence 和游标查询。
- [x] RunTurn 改为后台执行并返回 accepted。
- [x] 增加 Session Event Subscription SSE 路由、回放和 heartbeat。

## UI

- [x] 增加独立事件订阅客户端、游标和重连。
- [x] 发送命令与响应订阅解耦。
- [x] 保持 Workspace/Session 切换隔离。

## 验证

- [x] Gateway 单元与持久化 E2E 通过。
- [x] media-cli 双订阅 HTTP/SSE E2E 通过。
- [x] 双页面真实浏览器广播验证通过。
- [x] Go Harness 检查与 UI test/build 通过。
