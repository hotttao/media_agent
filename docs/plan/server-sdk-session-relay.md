# 第二阶段 Server SDK 与会话中继

对应原理005，规格见 ../spec/server-sdk-session-relay.md。

- [x] 基于当前IDL定义Session、Turn、Message、AgentRequest类型。
- [x] Fetch SessionRelay：应用登录校验、服务端Workspace解析、Session归属检查、固定路由和JSON限额。
- [x] 同源写请求限制、响应脱敏、SSE游标、取消与不重试写操作。
- [x] 业务接入样例、SDK测试和TypeScript声明检查。
- [x] 001运行验收：启动真实Agent运行环境，SDK创建Session并执行真实Turn，验证历史恢复、中断与续接。

2026-11-08验证：node --test通过8项；tsc --noEmit --strict --lib es2022,dom index.d.ts通过。现有Gateway配置引用的127.0.0.1:3001与3003的health请求均无法连接，本阶段尚未完成真实运行验收。当前测试中的上游流是确定性fixture，不能据此认定真实模型运行可用。Server SDK新增功能不要求修改Go业务领域或数据库schema。

2026-11-09进展：已启动本地Codex Runtime与RabbitMQ，Codex /health返回200。通过实际部署的Auth、Control、Gateway和当前SDK SessionRelay创建Codex Session、读取历史、查询会话列表均成功；不再以运行端口不可达作为阻塞。此前误判需要另提供 API Key；个人 Codex OAuth 本来受支持。后续经用户授权导入到测试应用托管用户，应用接入缺口已经补齐。真实推理验收仍未完成。


- [x] 应用个人模型连接 CRUD、Ensure 引用与 SDK 类型，验证越权、撤销及 CAS。
- [x] 使用个人 Codex OAuth 完成应用 SDK 真实 Turn 验收。

2026-11-09：Control 连接接口、SDK 与 Ensure 引用已部署。真实 HTTP 导入、列表、绑定与幂等重试、撤销通过；真实 SSE 断开取消异常已修复并补回归测试。当前静态 Codex Runtime 关闭 metadata，带 metadata context 的 Turn 在模型调用前失败（invalid_turn_input）；静态 Gateway SubmitTurn 也不携带个人连接，必须使用受管理 Runtime 的实例注册与逐轮凭据加载链路完成后续验收。未把失败 Turn 或协议测试标记为模型推理通过。

2026-11-10验收完成：恢复现有 Docker Controller、Broker、模型代理及 Gateway 受管理运行配置；使用授权的个人 Codex OAuth 连续完成两轮真实回复，并验证历史、Last-Event-ID 续接与第三轮准备期间中断（turn.interrupted）。已修复 native prepare 取消及 bridge 清理；Python 125 passed / 12 skipped，14 条 import-linter 契约通过，mypy 通过。记录见 docs/review/server-sdk-runtime-smoke.json。

## Go SDK（2026-11-10）

- [x] 按 architecture 分层建立独立 `packages/platform-go-sdk` module，保留 JS SDK。
- [x] 复用 IDL 生成 Go client，生成 DTO 仅存在于出站适配器。
- [x] 应用用户凭证、Agent、个人模型连接、会话、SSE、中继与 Caller 验证。
- [x] SDK 依赖边界检查、协议组合测试、Caller/缓存测试和使用文档。
- [x] Go SDK 真实个人 Codex OAuth 推理、历史、续传、中断与连接撤销验收。

实现说明与限制见 `docs/review/platform-go-sdk.md`；真实运行记录见 `docs/review/platform-go-sdk-runtime-smoke.json`。Go SDK 尚未对外发布。
