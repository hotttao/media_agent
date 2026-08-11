# media_agent_gateway 最小 Chat 落地计划

## P1 · Gateway 事实存储与 API

- [x] 使用 `harness/tools/new_server` 初始化 Go/Hertz Gateway，并按 Harness Go 分层实现。
- [x] 添加 PostgreSQL migration、ORM model、repository 与事务边界。
- [x] 实现 Cookie 身份解析、默认 Session、历史 Message API。
- [x] 实现 Runtime SSE 代理、完整事件与 Message 持久化。
- [ ] 覆盖幂等、归属校验、Runtime 失败和事件顺序测试。
- [x] 按 Harness E2E 规范添加 `media_agent_gateway/e2e` testcontainers 持久化测试与 `media-cli/e2e` HTTP/SSE 场景。

## P2 · Codex 默认运行环境

- [x] media_control 未配置时 Hooks 使用合理 mock 默认值成功启动。
- [x] 添加 Codex Runtime Dockerfile 与 auth.json 构建准备脚本。
- [ ] 验证镜像内 auth 路径、权限和 Codex 启动。

## P3 · UI Chat

- [x] 登录后的受保护首页替换为 Chat 页面。
- [x] 实现默认 Session、历史加载、消息发送与 SSE 消费。
- [ ] 覆盖加载、流式回复、错误和刷新恢复测试。

## P4 · 联调验收

- [ ] 启动 PostgreSQL、media_auth、Gateway、Codex Runtime 与 UI。
- [x] 登录后完成真实 Codex 对话，并验证同一 Session 双页面广播。
- [x] 通过真实 PostgreSQL testcontainers 查询证明 User/Assistant Message 与 Universal Event 完整保存。
- [x] 运行 Harness 强制检查、前端 build/test 与端到端场景。
