# Codex / Claude Code 双 Agent Runtime 落地计划

## P1 设计与 Control

- [x] 统一 Agent Type 与 Provider API Key 术语、秘密托管 ADR、Domain/Workflow。
- [x] 扩展 Control 领域校验支持两个 Agent Type并补齐不变量测试。
- [x] 核对现有 string IDL/OpenAPI兼容双枚举并增加 media-cli Control E2E。
- [x] 用 PostgreSQL 集成测试验证密文、留空保留与公开不回显。

## P2 Claude Runtime 与 Gateway

- [x] 实现 Claude CLI 受管理进程、stream-json 翻译、interrupt 与单元测试。
- [x] 增加 Claude Code Dockerfile、共卷 HOME 和 Compose 服务。
- [x] Gateway Runtime Registry 注册两个 Agent并验证路由选择。
- [x] 用 fake Claude CLI 完成 Gateway→Agent→持久化消息 E2E。

## P3 UI

- [x] 抽出 Workspace Agent Type 与表单规则到 `domains/workspace`。
- [x] 启用 Codex/Claude Code 选择并动态显示聊天文案。
- [x] 覆盖创建、编辑和关闭清 Key 的组件测试；失败态沿用既有错误分支。
- [x] 更新并运行 Playwright Workspace 旅程。

## P4 门禁与提交

- [x] 运行 Go 测试、arch-check、Python Ruff/Mypy/Pytest/分层检查、UI lint/build/test/dep-check。
- [x] 真实构建 Claude 镜像并验证 `/workspace/.home/.claude` 跨重启保留。
- [x] 运行最终 media-cli 双 Agent E2E并准备独立提交。

## 风险与对策

- Claude stream-json 版本漂移：翻译器只依赖公开稳定字段，未知事件忽略并保留诊断测试 fixture。
- API Key 泄漏：响应契约、日志扫描与黑盒 E2E三层验证；Agent 不写 auth 文件。
- 同卷并发：Compose/runtime controller 将所选 Agent 作为互斥实例；Domain 保留该不变量。
- 无真实 Provider Key：核心 E2E 使用协议级 fake CLI，镜像只验证官方 CLI 可执行。
