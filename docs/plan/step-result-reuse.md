# Step Result Reuse 落地计划

依据 [Spec](../spec/step-result-reuse.md)。与画布内运行向导并行作为本轮需求，不扩展 Agent Token 或历史迁移范围。涉及 Workflow 证据及 Materials 校验接口，按跨服务规范维护。

- [x] 区分普通新执行复用、原生失败继续、历史 Resume；记录目标领域规则和流程。
- [x] P1：规范化输入摘要及媒体 checksum 解析端口，边界单元测试。
- [x] P2：来源证据 migration、仓储端口、Finalize 登记接线及幂等/隔离测试。
- [x] P3：内部检查任务、IR 编译 SWITCH、分支统一输出、冻结定义兼容门禁。
- [x] P4：查询复用来源、节点策略配置、运行展示，类型/多语言/组件测试。
- [x] P5：按 Harness E2E 隔离验收 Conductor 命中不派发生成任务、未命中完整执行、审批重新生成及下游复用结果。真实历史产物缺少来源证据时不得伪造匹配。

依赖：P2 依赖 P1，P3 依赖 P1/P2，P4 依赖查询契约，P5 依赖全部接线。验收前不得宣称自动复用可用；不在修复取消导航时启动生成 Worker 或创建测试 Job。

## 当前接线与证据

以下按实施顺序记录；早期“尚未完成”的描述仅指当时阶段。最终状态见末尾验收收口。

- 2026-10-11：checksum/指纹单测、Materials 全量单测通过；隔离 PostgreSQL E2E 验证来源证据并发重放、四维隔离、不可改写约束及事务回滚，不涉及用户数据库。
- 内部检查任务已接入 `cmd/internal_worker`；新发布冻结检查 TaskDefinition。Schema 4 编译为 Prepare → Check → Conductor SWITCH → 原有生成/审批 DO_WHILE → Finalize；冻结 Schema 3 和历史 Resume 不改变计划。
- Finalize 已接线：命中核对真实已完成 Check 的输入/输出，再校验来源 Asset，不生成新产物；未命中按最后成功 Invoke 的实际输入登记证据，不能使用审批重新生成前的摘要。
- Finalize 单测覆盖命中无登记、技术重放、伪造 Check 拒绝、来源删除与存储故障区分、反馈后实际输入摘要和来源身份稳定。Conductor 真实执行和查询展示尚未完成，P2/P3 保留未勾选至接线整体门禁通过。
- Check 后来源删除：Finalize 明确失败，由 Conductor 保持失败/重试语义；不在服务中悄悄重排生成分支。临时存储故障为可重试错误，不能伪装为未命中。
- 查询/展示已接线：同次授权的 Conductor 快照派生 `result_source` 和源 StepRun；当前已完成 Finalize 与本地投影产物一致才展示，不新增数据库状态列或额外引擎读取。来源标注、无伪审批/空结果的组件测试（8 项）通过，后端 Job 查询和 Handler 单测通过。
- 真实 Conductor 协议门禁 `TestConductorCompiledReuseBranch` 两项通过（2026-10-11）：全命中零 Capability/HUMAN；上游命中结果进入下游提示词，下游未命中执行并审批重新生成，反馈和最终单输出正常。隔离别名 Task 及显式测试结果不调用 Provider；此门禁验证真实编译/引擎表达式与循环，不替代真实 Materials/数据库/内部 Worker 全链验收，P5 仍未完成。
- 全链文本验收 `TestResultReuseInternalWorkerRoundTrip` 通过：真实 Runner/Check/Finalize、真实 Materials Hertz 路由及两套独立临时数据库，不 Mock 资源登记或来源校验；首次生成、第二次零生成复用、force_new 独立产物，Materials 回执数量与真实生成次数一致。
- 图片验收 `TestResultReuseBinaryStorageRoundTrip` 通过（2026-10-11）：使用显式测试 Storage 配置、真实 S3 上传/HEAD/解析 SHA-256、Materials 当前来源校验，复用与 force_new 同样通过。只 Mock Capability 的返回值，不调用真实模型。唯一随机路径测试对象与两套临时数据库由测试清理，原用户历史执行未变。
- Materials、Workflow 和 Internal Worker 镜像已更新到当前测试服务；两服务 `/health` 返回 200，Internal Worker 已启动；未启用可观测性栈。最后补充 Schema 4 原生 Retry 与冻结 Schema 3 技术重试回归后进行完成审计。
- Schema 4 并行复用图原生 Retry 通过：完成的 Prepare/Check/复用分支保持原 Task ID，失败的生成继续，审批重新生成携带反馈。冻结 Schema 3 并行失败继续及 Finalize 登记后 TCP 响应丢失的原生自动重试也通过；后者不依赖 Job GET，不重复登记产物。
- 公开 API Token 发布验收发现两项真实问题并收口：旧无企业模板不具备绑定新企业 Workflow 的授权，新建页候选已与后端对齐；复用 Identity 嵌套结构体在 JSON/JSONB 解码后键顺序不同造成定义 hash 冲突，已只在新 schema 4 编译参数中规范成 JSON 值，不改变旧定义哈希算法。公开发布验收待新镜像部署后复验，当前不能勾选 P5。

## 最终验收收口（2026-10-11）

- `TestWorkflowReusePublicationGateway` 通过：使用现有 API Token 经 LAN 统一网关，创建并发布独立企业模板/Workflow，真实 Conductor 注册四个复用检查及 SWITCH，冻结 identity/provider_input 完整。不模拟登录、不启动生成任务；所有测试定义/模板按资源生命周期归档，原定义及历史执行未修改。
- 定义序列化后哈希一致回归、Compiler/Publication/Finalize/Reuse/Job 查询测试通过；Workflow 架构检查通过，`git diff --check` 通过。
- 前端来源展示/纯文本/无伪审批测试、企业模板选择规则测试通过；最新 typecheck、dep-check、i18n key 检查、standalone/plugin 构建通过。
- 最新浏览器登录与三个只读业务场景均通过：节点向导图片及搜索、复核、宽窄容器；商品预填及取消回商品；设置输出选择与确认返回画布。无 Submit/Resume/模板/定义写入。共享机器 16 CPU、负载约 62，冷登录默认 30 秒预算曾不足；诊断使用 `E2E_AUTH_TIMEOUT_MS=120000`、应用准备 60 秒及仅在实际 `ERR_NETWORK_CHANGED` 时的一次业务操作前 bootstrap reload。默认预算和业务命令不改，不能把此记录当接口性能达标。
- 当前 Materials、Workflow、Internal Worker 已部署新镜像并保持启动，两个 `/health` 及 Vite 5173 返回 200；未启用可观测性栈。
- 全部本轮功能验收完成。复用需新发布的 schema 4 定义；冻结 schema 3/历史 Resume 不改写。旧产物无完整输入证据或无可信 SHA 时不猜测复用，跨 owner/Workspace 共享不在本轮范围。
