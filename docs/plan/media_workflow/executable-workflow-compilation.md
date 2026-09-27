# Executable Workflow 编译落地计划

## P1 · 领域类型与纯函数

- [x] 增加 Invocation Mapping 表达式与校验。
- [x] 增加 `ExecutableWorkflowIR` / `ExecutableStepIR`。
- [x] 实现确定性 Template Renderer 与 digest。
- [x] 单元测试覆盖动态 context、映射顺序和错误边界。

## P2 · Semantic Compiler

- [x] 扩展 Template/Capability 冻结快照。
- [x] 支持显式 Workflow Input 引用，并将 API 统一为 `slot_bindings`。
- [x] 编译 Slot Binding、Context 与 Invocation Mapping。
- [x] required 覆盖和类型检查；生成确定性 IR 结构。

## P3 · Conductor Backend Compiler

- [x] 以 IR 替换当前直接消费 Workflow 的 compiler 入口。
- [x] 自动插入 `media_render_template_v1`。
- [x] 生成具体 Capability Task 与有序参数引用。
- [x] 保留拓扑波次、唯一输出和稳定 Definition Hash。

## P4 · 持久化与发布

- [x] migration：Capability、Template Capability 引用、Invocation Mapping、Compilation。
- [ ] Publication Outbox 与失败注册后台重试。
- [x] 发布状态机与快照冻结。
- [ ] Conductor 注册冲突 hash 比对和自动重试。
- [x] 只有 registered compilation 可 SubmitJob。

## P5 · Renderer Worker 与联调

- [x] 独立 Worker 进程轮询 `media_render_template_v1`。
- [x] 接入纯 Renderer，完成 task complete/fail 与确定性 digest。
- [ ] internal worker 的 trace_id 透传和任务指标。
- [x] 注册内部 Task Definition。
- [x] 使用真实 Conductor 验证 `业务 Slot -> prompt/images[] -> Capability`。

## P6 · 客户端 Media Worker 与旧流程样本

- [x] 新建客户端 `media_worker`，完成直连 Conductor 的早期联调验证；该传输实现已被 ADR 0017 取代，不作为生产完成项。
- [x] 实现 `media_ai_fixture` Mock Profile，冻结一个已完成视频流程的结果与 provenance。
- [x] 迁移七阶段 `model-image -> style-image -> first-frame -> video -> clip -> ai-fill -> publish` Workflow、Templates 和 Capability 引用。
- [x] 只通过 API 导入并执行一个完整样本；验证 Job、Conductor tasks 与最终视频结果。
- [ ] 等用户确认后再迁移全部旧数据。

## P8 · 人工审批闭环

- [x] IR v2 冻结候选数量与是否需要审批。
- [x] Backend Compiler 在需要审批的 Capability 后插入 HUMAN task。
- [x] 对账 HUMAN input 为 StepRun candidates/need_confirm，并从 UI 确认唯一候选。
- [x] 确认后完成 HUMAN task，Finalize 登记 Generated Asset 并继续 DAG。
- [x] 重新生成从当前 Capability task rerun；Cancel/Retry 同步控制 Conductor execution。

## P7 · 可直接使用的 Job 输入表单

- [x] Input Schema 增加 asset_kind，并校验同名 material/asset input 的类别一致性。
- [x] 回填既有 Workflow 的 Input Schema asset_kind。
- [x] 前端按 Workflow Input Schema 查询 Product、Virtual IP、Material 与 Generated Asset 候选。
- [x] 用资源选择表单组装 JobInput，移除业务用户可见的 JSON 编辑器。
- [x] 覆盖加载、空态、失败、必填校验、Workflow 切换和窄屏布局。
- [x] 通过组件/单元测试和实际账号 API 联调验证，不执行第二条旧 Workflow 样本。

## P9 · Worker Gateway 与 Conductor 安全代理

- [x] 扩展 Service Token：Organization 绑定、Worker scopes、expiry、introspection Principal，并保持明文只返回一次。
- [x] 配置 Traefik + Oathkeeper/Edge Auth 的 Worker 路由，内省 Service Token 并签发短时 audience=`media-workflow` Internal Identity JWT。
- [x] 拆分浏览器与 Worker 认证中间件；Worker API 不再要求 Kratos Cookie，并统一 `sub=business_user_id`。
- [x] 新增 Worker registration/poll/heartbeat/complete/fail API；Worker 只声明 Capability Version。
- [x] 新增 `worker_dispatches`、`worker_result_inbox`、状态机、唯一约束与租约扫描器。
- [x] StartWorkflow 使用 `taskToDomain`；MVP 为每个 User 派生 Execution Domain，禁止客户端 domain 与 `NO_DOMAIN` fallback。
- [x] 实现 Capability 到 taskDef 的服务端映射与受控 long-poll；排除 Renderer/Finalizer。
- [x] 实现结果 Inbox + Outbox/Adapter，先返回稳定 receipt，再幂等更新 Conductor。
- [ ] 实现 Object Path 到即时 presigned GET/PUT URL，校验上传摘要，拒绝任意结果 URL。
- [x] 修改 `media_worker`：删除 Conductor URL/client，改用 Gateway URL、Service Token、dispatch/heartbeat/result receipt。
- [ ] 覆盖 Token 撤销、断线恢复、重复完成、旧 attempt、租约过期、Job 取消、Conductor 暂时不可用和 trace_id 跨服务关联测试。
- [ ] 用一个 `media_ai_fixture` 完整流程验证客户端无法访问 Conductor，确认后再开始全量旧数据迁移。
