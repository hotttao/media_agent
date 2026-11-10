# 应用 SDK 个人 Codex 接入验收

应用个人连接接口与 SDK 已完成并部署到本机 Control/Oathkeeper/Traefik。复用现有 Model Connection 聚合和加密存储，不创建新服务或表。owner 仅取在线验证的应用凭证；Ensure 引用连接并沿用 Workspace 的 owner/active 校验，连接 ID 参与幂等摘要。

验证通过：

- Control `go test ./...`；独立 PostgreSQL 的应用绑定测试实际执行，包含并发幂等、回滚、跨应用/用户/部门隔离、连接归属、API Key 互斥、错误 Runtime 类型、连接替换冲突及撤销。
- SDK 10 项 Node 测试与严格 TypeScript 声明检查；生成 Control clients 编译通过。
- 经真实 Ory 入口、Auth、Control 和当前 SDK，以用户授权的本机个人 Codex OAuth 导入连接、列表、绑定与重复 Ensure、CAS 更新和撤销；混合认证返回400，不存在连接返回404，旧版本撤销返回409，撤销后绑定返回400。
- 测试连接已撤销，成功验收应用已禁用且 Team Grant 已撤回。凭据未写入仓库、验收记录或控制台。
- SSE 取消回归：先取消上游 reader 再终止 fetch，避免主动断开触发 AbortError；已补模拟真实 fetch abort 行为的测试。

真实模型验收已通过（2026-11-10）：恢复已有 Docker Controller、product-broker、模型代理，以及 Gateway 的 metadata/runtime/workload 配置和 Agent authority 配置。部署当前 Codex Runtime 源码后，通过应用 SDK 和 SessionRelay 连续两轮得到 SDK_OAUTH_OK、SDK_SECOND_OK，终态均为 turn.completed；历史存在对应 assistant 回复，Last-Event-ID 重连只返回游标之后的事件。第三轮在原生准备期间中断，持久终态为 turn.interrupted。连接已撤销、应用已禁用、Team Grant 已撤回。详见 server-sdk-runtime-smoke.json。

本轮修复：ExecuteTurnFlow 跟踪 prepare 任务，中断创建/恢复阶段时取消准备、等待 bridge 清理，再提交 interrupted；正常关闭取消仍向上传播。新增新会话及已有句柄准备取消回归，并验证创建/加载取消均销毁未注册 bridge。受管理执行保持固定元数据版本、实例身份与逐轮凭据获取。

Python 验证：125 passed / 12 skipped（外部集成环境测试未启用）；ruff check、Python 源码格式、mypy（151 files）、import-linter（14 contracts）通过。全目录新版本 ruff 还会检查既有架构 Markdown 中的代码块，存在原有格式差异；vulture 报告既有 ordered_dispatcher 的 SDK 回调覆盖方法 _dispatch_notification 未使用，未将这两项标记为通过。

个人 Codex 接入验收时，架构检查报告 Control 原有的15处 service→具体DAL依赖。随后已将这些 service 的事务接口改为消费侧声明、由初始化代码注入 DAL 实现；重新执行 Control 架构检查通过。`gofmt` 与 `git diff --check` 通过。明细见同名 JSON。
