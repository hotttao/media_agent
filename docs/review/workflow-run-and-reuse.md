# 画布内运行与结果复用完成审计

日期：2026-10-11。范围以 [运行向导 Spec](../spec/ui_design/workflow-run-wizard.md)、[结果复用 Spec](../spec/step-result-reuse.md) 及对应 Plan 为准；不扩展 Agent Token、历史全量迁移、真实 Provider 实现或系统负载优化。

| 要求 | 实现与权威验证 | 结论 |
|:---|:---|:---|
| 画布按真实节点收集入口、共享入口一次选择、搜索图片、回退及最终提交 | `run-input-groups` / `run-wizard` 规则与组件测试；真实网关 `workflow-run-wizard.spec.ts` 完成四组输入、复核、图片尺寸、窄容器与零提交断言 | 完成 |
| Product 可选兼容 Workflow、预填及取消回原 Product | Navigation Port/实例测试及 `product-workflow-run-wizard.spec.ts`；当前商品详情和 Product Tab 恢复，零提交 | 完成 |
| 设置可选最终输出、有确认按钮、空节点前置说明 | WorkflowForm 组件测试及 `workflow-settings-cancel.spec.ts`；草稿保留，不隐式保存/发布 | 完成 |
| 保留 Slot/plugin 边界及中文默认/英文支持 | Host Adapter 和实例状态；dep-check、i18n key 检查、standalone/plugin 最新构建均通过 | 完成 |
| 相同实际输入优先复用、force_new 独立生成 | `reuse_task` 指纹及边界测试；`TestResultReuseInternalWorkerRoundTrip` / `TestResultReuseBinaryStorageRoundTrip` 使用实际 Check、Finalize、Materials 路由/仓储与独立临时数据库，图片使用真实 S3 SHA-256；第二次零生成、第三次独立产物/回执 | 完成 |
| 源资源、权限边界、失效/故障、晚到结果安全 | 来源核验和 Finalize 重核验测试；同企业/Workspace/owner/Scope 仓储隔离、并发重放、回滚与 DB 不可改写约束 E2E；缺 SHA 不匹配、存储故障不可视为 miss | 完成 |
| 下游/并行/审批再生成与原生 Retry | 实际 Conductor `TestConductorCompiledReuseBranch`：并行分支、下游真实绑定、HUMAN 反馈及原生失败继续保留完成 Task；冻结 schema 3 并行 Retry 和登记后 TCP 响应丢失自动重试通过 | 完成 |
| 新定义可经公开接口发布，冻结版本不改写 | `TestWorkflowReusePublicationGateway` 经现有 API Token 和 LAN 网关注册四个原生复用分支；JSON 持久化前后哈希回归；schema 3 与历史 Resume 兼容编译测试 | 完成 |
| 来源展示是真实投影，不增加调度器/查询登记 | Job 查询/Handler/StepRunCard/JobShow 测试；只消费当前完成 Finalize，重复利用同次授权快照，不新增引擎 GET、业务重排或注册副作用 | 完成 |
| 实际测试服务可用 | 新版 Materials/Workflow/Internal Worker 运行，两个 `/health` 和 `http://192.168.2.41:5173` 返回 200 | 完成 |

生成返回值仅在明确测试 Capability 中 Mock，不宣称调用了真实模型。真实图片测试对象与临时数据库已清理，公开发布测试定义/模板已归档；原历史 Job 不变。观察栈未启用。

最新浏览器轮次：登录和三个业务场景 `4 passed`。共享机器负载导致默认冷启动预算不足及 Chromium 网络变更；诊断仅延长准备预算，并允许实际网络变更后的业务操作前一次 reload，不重发登录/提交，不代表性能指标通过。最新前端构建仅有既有 FlowGram chunk 大小提示。

复用对新发布 schema 4 生效；旧冻结定义不能原位升级。缺少真实输入证据的历史产物不能自动匹配，原生历史 Resume 继续使用既有已完成前缀。本轮范围无剩余开发项。
