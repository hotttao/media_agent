# 企业素材与工作流平台实施计划

对应已确认 spec：[enterprise-media-production.md](../spec/enterprise-media-production.md)。

## Phase 0：契约冻结与基线

- [x] 确认 DAG 波次、节点审批策略、同 StepRun regeneration 三项决策，将 spec 标为 Accepted。
- [x] 为旧 `docs/spec/media_materials/impl.md`、`docs/spec/media_workflow/impl.md`、认证与 UI spec 标注被本 spec 替代的边界，消除双重事实源。
- [ ] 固化 IDL 错误码、幂等 header、Internal Identity JWT audience 与服务间调用约定。
- [ ] 记录当前数据库、API、UI 与执行链路基线；建立 feature flag：`ory_auth`、`keto_enforce`、`conductor_execution`、`shell_context_v2`。

验收：文档链接无断链；DDD、IDL proposal、spec 使用同一术语；所有切换都有 Workspace 级回滚开关。

## Phase 1：Ory 与企业权限底座

### 部署与 media_auth

当前进度（2026-09-17）：仓库内 Compose 已具备 Kratos/Oathkeeper/Keto、文件化 Traefik 路由、Oathkeeper RS256 Internal JWT 和业务服务验签；`users.kratos_identity_id`、Organization/Membership/Entitlement、Team Entitlement/状态及授权 Outbox migration 已落地。Organization API 已完成；Team 已固化企业归属，创建要求 active Organization 的企业管理员，添加成员和接受邀请要求目标用户先属于企业。Team Entitlement API/Outbox、Workspace 归属、Keto Projection Worker 与灰度 enforce 尚未完成，所以下列整项暂不提前勾选。

- [ ] 在 compose/部署仓库加入 Kratos、Oathkeeper、Keto 的数据库、migration、健康检查和仅内部开放的 admin endpoint。
- [ ] 定义 Kratos identity schema、browser login/recovery flow 和 Oathkeeper access rules/JWT mutator。
- [ ] 为 `users` 增加唯一 `kratos_identity_id`；实现 webhook/import 的幂等关联，禁止新密码写入 media_auth。
- [ ] 完成 Organization、Organization Membership/Entitlement、Team Entitlement、Team→Workspace 约束和 Outbox migration/repository/service/IDL。
- [ ] 实现 Keto namespace/relationship mapper、Projection Worker、checkpoint、重放命令与 lag 指标。
- [ ] 先 shadow 比较本地判定和 Keto 判定；差异为零后按 route 开启 enforce。

### 测试

- [ ] Kratos 登录、退出、恢复、disabled/deleted User 拒绝的集成测试。
- [ ] 企业管理员不能越过 Organization Entitlement；Team inherit/restricted、末位管理员、跨企业成员拒绝测试。
- [ ] Outbox 重投、撤权优先、Keto 全量重建和投影延迟测试。

回滚：Oathkeeper 保留旧认证 fallback 的短期灰度规则；关闭 `keto_enforce` 回到事实源判定，但不得回滚已经提交的企业数据。

## Phase 2：Execution Capability 与 Template 迁移

- [ ] 在 `media_workflow/migrations` 增加 `execution_capabilities` 与 Template capability 引用；legacy route 字段先保留 nullable。
- [ ] 实现 platform-admin Capability CRUD、版本发布/归档和 Task Definition 预检。
- [ ] 盘点现有 `platform/gen_way/gen_type` 组合，生成显式 migration map；无法映射的数据保持 draft 并输出报告。
- [ ] Template 新写路径强制引用 published Capability，校验 input/output schema；读路径兼容已迁移旧版本。
- [ ] 为每个首批 Capability 定义 Worker contract fixture 和 schema compatibility test。

验收：所有可发布 Template 都固定 Capability version；无 UI 自由输入队列名；密钥和临时 URL 不出现在数据表/API。

## Phase 3：Conductor 编译器与发布链路

- [ ] 增加 `workflow_compilations`、发布 Outbox、状态 migration 与 repository。
- [ ] 实现纯函数 compiler：DAG 校验、Kahn 排序、wave 分组、reference name 清洗、canonical JSON/hash。
- [ ] 注册固定 `media_step_execution_v1` Definition 及必需 SIMPLE Task Definition；名称带平台 namespace/version。
- [ ] 实现 Conductor Adapter：register/get/compare hash、start/query by correlationId、complete WAIT、terminate。
- [ ] PublishWorkflow 改为 `draft -> publishing -> published|publish_failed`；只有 registered compilation 可 SubmitJob。
- [ ] 增加 reconciliation job：修复 publishing、启动结果未知、远端定义漂移和孤儿 execution。

### 编译器测试矩阵

- [ ] 单节点、线性、菱形、多根单 sink、宽并行、非法环、多 sink、悬空 binding。
- [ ] 同输入生成 byte-identical canonical JSON/hash；同 name/version 同 hash 幂等，不同 hash 明确冲突。
- [ ] 特殊字符/长 step key 的 reference name 唯一、稳定且可追溯。
- [ ] 注册超时、Conductor 重启、Outbox 重投不会产生不一致 published 状态。

回滚：关闭 `conductor_execution` 后禁止新 Conductor Job，已启动 execution 继续由 Adapter 收尾；Definition 不删除。

## Phase 4：Step Fragment、Worker Gateway 与生成闭环

- [ ] 实现 `resolve_reuse`、Capability dispatch、`persist_candidates`、review WAIT、`finalize_candidate` 的 Worker/handler。
- [ ] 在 `media_gen` 固化 canonical reuse fingerprint、`reuse_if_available/force_new` 与成功结果可达性检查。
- [ ] 实现 Worker Gateway 的服务身份、Capability/Runtime Environment/Workspace 授权、poll/heartbeat/complete。
- [ ] SubmitJob 创建全量 StepRun + Start Outbox；Conductor correlationId 固定为 job_id。
- [ ] 实现 accept/regenerate 命令、Review Decision 唯一约束、CompleteWait Outbox 与 stale attempt 防护。
- [ ] 实现 `approval_policy=when_multiple_candidates|always`：多 Candidate 强制 WAIT，单 Candidate 按节点策略决定自动确认或 WAIT。
- [ ] 实现 cancel、timeout、dead-letter、WAIT 超龄告警与手工 reconciliation 命令。

### 故障测试

- [ ] Worker 在“业务提交后、Task complete 前”崩溃，重投不重复创建资源。
- [ ] Start/CompleteWait/terminate 响应丢失后通过查询收敛。
- [ ] Candidate 属于错误 attempt、重复 accept、accept/regenerate 竞争均被稳定拒绝。
- [ ] regenerate 生成新 attempt、携带 feedback、强制 cache miss，旧 Candidate 可审计不可再决定。
- [ ] Conductor/Worker/media_workflow 分别重启后 Job 与 WAIT 恢复。

## Phase 5：UI Shell Context 与业务页面

- [ ] 在 `ui_workflow/src/foundation/lib/host-shim` 定义 ShellContextValue、RouteIntent 与契约测试。
- [ ] standalone adapter 接入 Organization/Team/Workspace 路由和 Menubar；旧 `/w/:id` 解析层级后重定向。
- [ ] plugin adapter 映射宿主 context/navigation，不引入第二个 Desktop、Router 或全局导航。
- [ ] Organization/Team/User 管理页接 Ory 与 media_auth；功能授权编辑器展示企业上限和 Team 有效交集。
- [ ] Materials App 完成既有资源 CRUD/共享；所有请求携带 Workspace 上下文并由服务端授权。
- [ ] Workflow App 完成 Capability 管理、Template、DAG、发布/Compilation 状态、Job 监控。
- [ ] Candidate Review 展示 attempt、全部候选、accept、feedback/regenerate；提交中和已决定状态防重复。
- [ ] App 级动作走 App Header Portal；页面跳转只走 Navigation Port。

### UI 验收

- [ ] `yarn typecheck/build/test` 同时生成 standalone 与单 bundle plugin。
- [ ] 两种宿主共享 L2/L3 业务组件；dependency-cruiser 无 L0-L3 逆向依赖。
- [ ] 切 Organization/Team/Workspace 时 Query cache、Profile、选中资源不跨边界泄漏。
- [ ] 窄屏单 Slot、Dock、Header Portal 和 Candidate Review 可键盘操作并有清晰 loading/error/empty 状态。

## Phase 6：灰度、迁移与退役

- [ ] 选择内部 Organization/Workspace 进行 Ory + Keto + Conductor 全链路 canary。
- [ ] 对相同 Workflow 在旧/新执行路径做 shadow 结果和状态时序比较，不对 Provider 重复扣费。
- [ ] 逐 Workspace 扩大 `conductor_execution`，监控 queue depth、error、WAIT age、projection/outbox lag。
- [ ] 将 standalone 与 plugin 分别发布、版本一致回滚；验证 plugin 只注册 Materials/Workflow 两个 panel。
- [ ] 停止旧登录和旧 Dispatcher 接收新流量；等待历史 Job 终态后再删除 legacy route/read compatibility。
- [ ] 输出运维手册：Definition 冲突、卡住 WAIT、孤儿 Task、Keto 重建、Worker 撤权和紧急停流。

## Definition of Done

- [ ] spec 第 10 节验收全部自动化或有可重复演练记录。
- [ ] 无浏览器到 Conductor/admin endpoint 的网络路径；secret 扫描和日志采样无泄漏。
- [ ] Job/StepRun 与 Conductor reconciliation 连续运行 7 天无无法解释的不一致。
- [ ] Keto projection lag 达到部署 SLO，撤权演练满足安全窗口。
- [ ] 任意一个灰度 Workspace 可单独回到旧路径，不回滚数据库、不删除审计历史。
