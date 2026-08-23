# Agent Session 配置切换落地计划

## P1 契约与 Gateway

- [x] SessionResponse 公开固定 `context_version` 并重新生成 model/client/OpenAPI。
- [x] 增加配置变化类型化错误，Run Turn 映射 HTTP 409。
- [x] Interrupt 按 Registration Agent Type 路由，不依赖 Workspace 当前配置。
- [x] 单测覆盖旧 Session 新 Turn 冲突、Interrupt 保持可用和响应投影。

## P2 UI

- [x] 保存后按 `configuration_version` 判断实际变化。
- [x] 配置变化自动创建/恢复并选中新 Session。
- [x] 旧版本 Session 显示只读提示和手动创建入口，禁止误发 Turn。
- [x] 单元与 Playwright 覆盖 Agent 切换、API Key 更新、部分失败和旧 Session。

## P3 E2E 与提交

- [x] Go HTTP E2E 覆盖 Workspace 更新后旧 Session 409、新 Session 可用。
- [x] 实际执行 media-cli 命令链路验证跨进程用户操作。
- [x] 运行 Gateway Go/arch-check、UI lint/test/build/dep-check。
- [x] 独立提交本功能。
