# Workflow 画布内运行向导 Plan

依据 [Spec](../../spec/ui_design/workflow-run-wizard.md) 与 [ADR](../../ddd/ui_design/decisions/0006-in-canvas-run-wizard.md)。不实现新的引擎或永久商品关联，不创建测试历史执行。

- [x] 核对 Slot/Navigation、Workflow/JobInput 契约、现有商品入口与搜索接口边界，固化 UI 旅程。
- [x] P1：稳定拓扑的节点输入分组与共享入口规则，添加纯函数测试。
- [x] P2：媒体搜索卡片、分页兼容清单与运行面板，保留原提交意图和冻结恢复表单。
- [x] P3：Workflow App 内准备/执行视图及商品详情可用工作流卡片，通过原 Navigation Port 预填；实例导航与商品入口组件测试通过，真实商品入口验收归 P5。
- [x] P4：状态/组件测试，typecheck、dep-check、i18n、standalone/plugin 构建。
- [x] P5：真实网关浏览器只读准备/取消验收、宽窄 Slot 检查与操作记录。不启动 Worker、不 Submit/Resume，不污染已清理数据。

## 新建工作流设置修复与取消导航

- [x] 无节点时明确说明最终输出的前置条件，禁用空选项控件。
- [x] 提供确定并返回画布操作；不隐式保存或发布，不丢失草稿。
- [x] 组件测试验证有节点时选择最终输出及确认后的草稿保留。
- [x] 取消回到源 Workflow / Product，旧 plugin 端口缺失时回到源 Workflow；组件及 Navigation Port 测试通过。
- [x] 真实网关浏览器回归设置选择及取消，不写定义或创建任务（2026-10-10，LAN Origin）。

验证记录（2026-10-10）：11 项组件/导航测试通过，typecheck、dep-check、i18n key 检查通过。真实浏览器回归尚未通过：初次旧会话 401，本机测试 Origin 被 Kratos 拒绝；改用已授权 LAN Origin 后完成登录，但 Chromium 的 ERR_NETWORK_CHANGED 导致 Desktop 动态模块加载失败。未执行业务提交，不以调大超时当作验收通过。

后续验证：真实设置选择及取消已通过；新向导已在浏览器显示真实商品图片并通过可读性断言，完整准备验收仍在进行。轻量 Workflow List 不含权限动作，浏览器及商品候选先消费已授权摘要，选中后用 Detail 的 run/registered 核验；不得因为摘要缺字段而隐藏所有资源。新的 12 项规则/组件测试与旧 Resume/Rerun 回归通过；两种构建已通过一轮，最终改动仍需收口复验。

最终浏览器记录：`workflow-settings-cancel.spec.ts`、`workflow-run-wizard.spec.ts`、`product-workflow-run-wizard.spec.ts` 全部通过；真实图片自然尺寸非零、四节点逐步准备/复核、窄屏收起/展开、商品预填/取消回商品 Tab，均未发送 Job Submit/Resume。最新源码 typecheck、dep-check、i18n key 检查通过；standalone/plugin 最终构建仍在运行，P4 完成以前不得宣称 UI 全部门禁完成。

最终构建记录：最新 `yarn build` 已完成 typecheck、standalone 与 plugin，均退出 0。仅有既有 FlowGram bundle 大小提示，无构建错误。UI 门禁完成；结果复用是独立的后端编译及运行期落地项，按 [复用计划](../step-result-reuse.md) 继续。

2026-10-11 收口复验：最新版 standalone/plugin、类型、依赖、翻译检查通过。真实网关浏览器登录和三个业务场景再次全部通过（4 passed），仍不提交 Job。新建企业 Workflow 仅列当前企业模板，与实际 API 授权边界一致，不把旧无企业模板的读取权当绑定权；新增纯规则测试通过。共享机器冷启动/网络变更使用显式诊断预算，不更改默认预算、不重试业务命令。后端结果复用的真实图片/文本、原生 Retry、公开发布验收已完成，详见复用计划。
