# API Token 与 Worker Execution Target 落地计划

> 规格：[API Token 与 Worker Execution Target 实现规格](../spec/api-token-worker-execution.md)
>
> 领域依据：ADR 0017、ADR 0018，以及 `media_auth/api_token`、`media_workflow/execution_capability`、`media_workflow/execution`。

## P1 · Capability Target 与 IR

- [x] migration 增加 `execution_target`、`managed_pool_key` 和约束，旧数据回填 Personal。
- [x] Domain、DAL Model、Repository 映射 Target。
- [x] Semantic Compiler 快照并校验 Target。
- [x] Executable Workflow IR 冻结 Target，canonical JSON 保持确定性。
- [x] 单元测试覆盖非法 Target、Pool 约束和发布版本冻结。

## P2 · Personal/Managed Domain 调度

- [x] Job 启动按 Target 生成 Personal/Managed `taskToDomain`。
- [x] 检测相同 Task Definition 的 Domain 冲突，禁止 `NO_DOMAIN` 回退。
- [x] Worker Gateway 只接受 Personal Capability。
- [x] Dispatch 拆分 subject、actor、credential、instance 和 auth source。
- [x] 实现 Managed Worker 入口、配置 allowlist 和 Managed Domain poll。
- [x] 共用 lease、Result Inbox 与 Conductor result adapter。
- [x] 单元/集成测试覆盖双 Target 隔离与审计字段。

## P3 · Talos 基础设施与 Adapter

- [x] 按 Talos `v2alpha1` OpenAPI 锁定 Issue/Verify/Get/Rotate/Revoke adapter 契约；部署镜像版本仍需在接入 compose 时固定。
- [x] 在当前仓库增加 Talos OSS 配置和独立数据 volume。
- [x] 增加 `talos` typed adapter、配置 schema、超时、trace 和错误映射。
- [x] Adapter 使用 fake HTTP server 覆盖签发、验证、查询、轮换、撤销、超时和敏感信息不落错误。
- [x] 默认编码测试不启动 Talos；仅显式启动认证栈时运行。

## P4 · API Token 聚合

- [x] 新增 `api_tokens` migration、行模型、Repository 和领域不变量。
- [x] 新增 API Token IDL：create/list/rotate/revoke，以及 internal verify。
- [x] 实现 Talos Issue + 本地事务失败撤销补偿。
- [x] 实现 VerifyAPIToken，联查 User、Actor、Organization 和 scope。
- [x] 实现 revoke/rotate pending operation 与幂等恢复。
  - [x] revoke 使用持久 revoking 意图，后台分页恢复，并查询确认不确定结果。
  - [x] rotate 的持久操作、超时结果确认与未知 replacement Key 清理。
    - [x] migration、聚合操作实体、Token/操作事务 CAS。
    - [x] Talos metadata Rotate 与 Actor 分页 List adapter。
    - [x] 同步 Workflow、独立有界清理、后台重启恢复。
    - [x] 单元/PG 并发与失败测试，真实 Talos 丢失响应联调，HTTP/CLI 回归。
- [x] 生成 Hertz model/client/router/swagger，不手改生成产物。
- [x] 单元、DAL、HTTP E2E 覆盖生命周期与失败分支。
  - [x] 补齐创建时签发结果不确定、撤销补偿失败后的持久化恢复；不能只返回组合错误后丢失补偿意图。
    - [x] 独立 RecoverIssue Adapter：同 request_id 重放，仅返回元数据，严格核对 Actor/签发标识，单元与真实 Talos 契约测试。
    - [x] issuing/issuance_cleanup、nullable Key、期限与客户端去重迁移和聚合 CAS。
    - [x] Issuance Workflow、撤销未完成创建、后台重启恢复与 wiring。
    - [x] PG 并发/失败与真实丢失响应恢复测试；部署后 HTTP/CLI/浏览器回归。

## P5 · 委托身份与 Personal Worker 切换

- [x] Edge Auth 把 API Token 交换为短时 Internal Identity JWT。
- [x] JWT 支持 RFC 8693 `act.sub`，并校验 audience、TTL 和 issuer。
- [x] Worker Gateway 使用 `sub=User`、`act.sub=Worker`，不再把 credential 当 Actor。
- [x] `media_worker` 配置和文案由 Service Token 改为 API Token。
- [ ] 换发现有测试 Worker Token，删除本地 Hash 验证旁路。
  - [x] Worker Identity 术语/边界、migration、domain/DAL/service 与 owner/Organization/active 校验。
  - [x] 停用/删除事务与后台补偿；即时拒绝身份、精确 Actor/owner/Organization 撤销意图、晚到签发扫描，PG 与真实 Talos 测试。
  - [x] 公开身份管理和 Worker Token 签发 IDL/CLI。
    - [x] WorkerTokenProvisioning与受限scope、生成IDL/client/router/OpenAPI、薄handler、Talos独立身份服务接线。
    - [x] CLI新文件0600交付与失败撤销，HTTP/CLI各自验证归属、scope、停用/删除、重复和错误分支。
  - [x] 经公开CLI签发联调Worker Token，写入被Git忽略的media_worker/.env（0600），五类worker scope，不保存到文档/日志。
  - [ ] 经网关换发测试 Worker API Token、实际启动 media_worker 跑一个完整流程，覆盖禁用/撤销/跨组织拒绝。
    - [x] 稳定Worker API Token + 真实media_worker四节点单样本执行/结果收据门禁。
    - [ ] 已完成样本结果URL修复、页面图片/视频预览和生产对象上传验收。
- [x] 为测试 User 签发受限测试环境 API Token，Secret 仅保存到未跟踪的 `.env`；HTTP/CLI E2E 支持 Bearer API Token，不再要求预先模拟登录。
- [ ] 覆盖撤销、scope、Organization、Actor disabled 和 trace_id 联调测试。

## P6 · Agent Token 申请与代理

- [x] Control 稳定 Agent Identity 内部解析：按 Configuration ID 定位，只读投影归属/当前 owner 授权，服务身份保护、无 Provider Secret；单测与真实 HTTP 门禁。
- [x] Auth Agent ActorValidator：非缓存查询 Control，Create/Verify/Rotate 精确校验归属及当前授权；单测、部署后既有 User/Worker 链路回归。
- [x] Auth 内部 Agent Token Provisioning：专用 Gateway 服务凭证、owner事实派生、仅 agent.gateway scope、持久签发与重复命令拒绝重放Secret；单测/真实Talos接口门禁。
- [ ] 定义 Agent API Client/代理 IDL，不与 Agent Session Token 混用。
- [ ] Agent 创建后由 Gateway 幂等调用 media_auth 创建 Actor Token。
- [ ] Gateway 只回传一次 Secret，不持久化 Secret。
- [ ] Gateway 验证 Agent Token、校验 owner 委托并代理内部服务。
- [ ] Agent disabled/deleted/owner 解绑触发 Token 撤销。
- [ ] E2E 覆盖申请失败重试、代理调用和撤销。

## P7 · 删除旧 Service Token

- [ ] 明确 Platform Token 独立存储和调用方后，删除 `service_token` domain/policy/service/repo。
- [ ] 删除旧 IDL、路由、生成代码与 `tos_` 本地 Hash 工具。
- [ ] 删除旧表或改为只读迁移归档；历史 migration 保留可重放。
- [ ] 更新 OpenAPI、CLI、配置、部署和所有文档引用。
- [ ] 运行全仓 `make arch-check`、相关模块 `go test ./...` 和 E2E。

## 实施顺序与提交边界

```text
P1 -> P2 -> P3 -> P4 -> P5 -> P6 -> P7
```

每个阶段单独提交。P1/P2 不依赖 Talos，可先完成并保持现有 Personal Worker 可用；P3/P4 完成后再切换凭证，避免半成品认证链路阻塞 Workflow 开发。P7 只有在所有调用方和测试完成切换后执行。

## 2026-09-28 联调记录

- 为测试用户签发组织受限的 `api.access` Token，Secret 保存到未跟踪的 `ui_workflow/e2e/.env`（`E2E_API_TOKEN`），文件权限 0600。
- 修正 Oathkeeper introspection 的容器内服务地址，并通过 `ext` 把 credential、actor、organization、auth source 传入 Internal JWT。
- 修正 materials 的 API Token 分支缺失组织授权上下文；组织受限 Token 只能访问所绑定组织。
- `TestAPITokenGateway` 通过：用户、workflow、materials、非法凭据、跨组织拒绝、创建/轮换/撤销、scope 不足。
- CLI `api-token create/list/rotate/revoke` 与 `auth me` 实际执行通过，Secret 只写显式指定的新 `.env` 文件。
- 撤销支持 `revoking` 状态重试及重复撤销；持久 pending operation、轮换失败恢复、Worker/Agent Token 和 UI 优化仍待完成，以上记录不代表 P4/P5 整体完成。
- Managed Worker 增加内部进程入口及显式启用 profile，仅允许发布的 Managed Capability/Pool，不携带 API Token。Provider 通过配置的内部执行器接入，未配置真实实现时保持关闭。
- 真实 Conductor + testcontainers PostgreSQL 联调通过领取、续租、模拟模型结果写入 Inbox、结果重放及执行完成；结果并发/失败重试另有数据库集成测试。
- 联调修复 Conductor OSS 返回原始 UUID 的解析、5 秒长轮询上限和 Inbox 幂等回查错误；同一次 attempt 不能同时接受 complete/fail。
- API Token 轮换的本地提交改为对旧 Key ID 和 active 状态做 CAS；PostgreSQL 测试证明不能复活 revoking/revoked Token，两个并发轮换只能提交一个。
- Talos 错误响应正文、任意 error_code 和异常字段不再进入错误信息；缺失 Secret 的轮换响应被拒绝。持久 rotation pending operation 与自动恢复仍未完成，不以这些加固代替该验收项。
- Vite 补齐 Recovery/Settings/Verification 等 Ory 页面和明确的静态资源代理；浏览器验证注册、稳定 User、独立 Session 登出、恢复入口通过。FlowGram 拖动/节点详情/进入编辑单独复测通过；Docker/testcontainers 操作与浏览器测试串行执行，避免 Chromium `ERR_NETWORK_CHANGED`。
- 撤销恢复后台已部署：持久 revoking、keyset 扫描、失败记录不阻塞后页、错误后查询确认 Talos 撤销终态。PostgreSQL 集成测试（含 race）验证进程对象重建后恢复、响应丢失、分页和终态不重放；单元测试覆盖本地提交失败、查询失败及 Key ID 不匹配。
- 更新认证服务后 `TestAPITokenGateway` 通过；CLI 实际执行创建、轮换、撤销、重复撤销，Secret 文件权限为 0600。企微登录测试原有异步状态读取竞态改用通道同步，相关 service/workflow race 测试通过。轮换持久恢复仍未完成。
- 轮换恢复随后完成：rotating + 依附操作记录、同步事务 CAS、metadata 定位 replacement、旧 Key 撤销屏障、分页清理和后台恢复已部署。服务/Workflow/Talos adapter race 测试、9 项 API Token PostgreSQL/真实 Talos集成测试通过；真实 Talos 提交后丢弃响应并阻断即时清理，再重建 Workflow 验证仅靠数据库恢复，所有本次 Key 均 revoked，且晚到 Rotate 被拒绝。
- 更新认证服务后网关 API 生命周期与 CLI 创建/轮换/撤销/重复撤销回归再次通过；未把创建补偿意图丢失问题、Worker/Agent 切换或 UI 整体验收并入该完成项。
- 轮换恢复部署后的浏览器回归 6 项通过：登录 setup、注册入口、稳定业务 User、独立 Session 登出、密码恢复、FlowGram 节点拖动/详情/结构化编辑。仅证明这些旅程仍可用，不替代完整视觉与执行交互验收。

## 2026-09-29 联调记录

- 创建恢复契约已通过真实 Talos 验证：签发提交后丢失响应，以及恢复先于原请求到达，均只产生一个 Key；撤销后相同 request_id 重放仍返回同一 revoked Key，正常 Issue 不重发 Secret。此处仅完成恢复 Adapter，不代表创建意图、后台恢复或 P4 已完成。
- 重启后发现临时 Vite unit 消失；改为仓库维护的持久 user-service 模板，启动脚本 link/enable，显式 stop 同时 disable。当前机器启用 Linger，让用户服务管理器随主机启动，不依赖终端会话；仍采用 Vite :5173 → Traefik 后端的开发拓扑。
- 验证记录：media_auth 全模块单元测试、Talos/API Token service race 与架构检查通过；真实 Talos 恢复契约两种场景通过。5173 返回 200、持久 unit enabled/active、Linger=yes；网关 API Token HTTP 生命周期与认证/FlowGram 浏览器回归6项通过。没有执行整机重启验证，不以 enabled 状态宣称已完成重启实测；创建 Workflow 持久恢复和完整 UI 验收仍未完成。
- 创建持久恢复随后完成：migration0013、聚合 CAS、创建者范围内去重、Issuance Workflow、未完成创建的撤销和启动恢复已部署。单元 race 覆盖响应丢失、初始写库/激活提交/最终清理失败、取消、晚到成功、分页循环、跨 Actor/尝试拒绝、权威撤销确认和失败不阻塞后页。
- 15项 API Token PostgreSQL/真实 Talos 集成测试（race）通过；真实 Talos 提交签发后丢弃响应并阻断清理，再重建 Workflow，仅靠数据库恢复到 revoked。PG 验证 NULL Key、并发去重、创建者隔离、CAS、事务回滚、未完成创建撤销、期限与 migration 回滚保护。
- 部署确认数据库版本13且 dirty=false、认证容器运行新镜像且未重启。网关 HTTP 生命周期与重复创建409/不重发Secret通过；CLI实际执行创建/列表/轮换/撤销/重复撤销，Secret文件0600；认证与FlowGram浏览器回归6项再次通过。P4完成不代表P5/P6/P7或UI整体验收完成。

### Personal Worker 稳定身份与撤销基础

- 按 domain-modeling 先补充 Context Map、Worker Identity domain、Worker Access Revocation workflow、spec/plan，区分稳定身份、凭据、实例与任务。
- 实现0014迁移、domain/DAL/service与Organization-aware ActorValidator；签发、验证、轮换都检查当前身份授权，Agent未接权威前继续拒绝。
- 停用/删除与精确Worker Token撤销意图在同库事务提交；后台扫描非active Identity捕获晚到签发，现有签发/轮换/撤销恢复流程清理Talos。无Secret副本、无事务内外部调用、不复活停用身份。
- 单元/竞态/架构检查通过；隔离PG及真实Talos回归20项通过，包含5项Worker身份/撤销测试，验证即时拒绝与重建流程后Key revoked。GORM查询通过工具重生成，未手改生成文件。
- 部署数据库版本14、dirty=false，新镜像运行且RestartCount=0。网关HTTP生命周期、CLI实际创建/列表/轮换/撤销/重复撤销通过，Secret文件0600；认证与FlowGram浏览器回归6项通过，5173返回200。
- 公开Worker身份管理/签发IDL与CLI、测试Worker Token换发、真实media_worker单流程联调仍未完成；P5/P6/P7和UI整体验收仍未完成。

### Personal Worker 公开管理与签发

- 新增WorkerTokenProvisioning、公开Create/List/Get/Disable/Delete/CreateToken IDL与薄handler；owner/Actor/Organization只从可信User与Worker Identity派生，scope只能是五类worker能力的子集，空值默认为全部。身份Service与撤销意图独立于Talos配置。
- Hertz model/client/router及OpenAPI由工具生成；新增路由的两套HTTP注解保持实际路径一致。Traefik与Oathkeeper普通User管理规则同时加入worker-identities，并重启本仓库容器。
- media_auth与media-cli全量单测通过，workflow/handler/CLI竞态和架构检查通过。HTTP网关覆盖伪造字段拒绝、未知身份、scope隔离、JWT交换、User不能冒充Worker、重复签发无Secret重放、停用/重复停用/删除/重复删除、轮换拒绝和后台revoked收敛。
- CLI实际完成创建/读取/列表/签发/轮换/停用/重复停用/撤销/删除/读取历史；Secret文件0600。认证与FlowGram浏览器回归6项通过，5173可访问。
- 已通过公开CLI签发稳定Worker Actor的联调Token到media_worker/.env（0600且Git忽略），不替换User接口测试凭据。未启动Worker消费任务：只读列表发现历史RUNNING/NEED_CONFIRM Job，部分引用的Workflow返回404；单流程联调先核对队列与任务Capability，不擅自取消历史Job或启动批量执行。此观察不是阻塞，仍可继续安全排查。
- P5实际Worker单流程、P6/P7和UI整体验收仍未完成；本节不把公开接口门禁冒充完整Workflow执行门禁。

### Personal Worker 单样本执行门禁

- 真实media_worker使用稳定Worker Actor/API Token，经统一网关完成四个Capability；Job `job_4b90d93f-30bf-4248-b60c-e4e93d80c733` 四个StepRun均成功并持久化输出资源。四条Dispatch的auth_source为api_token、Actor正确；Result Inbox均submitted并保留mock/source/profile provenance。Renderer/finalizer仍由内部Worker执行。
- 原样本发布定义缺少Job/Step元数据；首次执行超时后经业务API取消，保留记录。使用现有迁移API工具发布新样本`wf_ed878c49-f6a7-47f4-8866-03420ac5ffec`，成功后归档旧样本。不修改旧Definition/Compilation或其他账号历史Job。
- 新增显式单样本HTTP门禁，已有Job观察模式不再次提交；迁移工具支持新样本名称，避免覆盖旧发布。
- 样本结果原地址192.168.2.89:3000不可达；已验证192.168.2.38上相同三张图片与视频均HTTP200，Worker改为可配置资源根地址。已完成Job仍保留原URL，页面结果预览与生产对象上传尚未验收，不能用状态成功替代可见结果验收。用户确认前不迁移其他Workflow。
- P5撤销/跨组织/trace_id整体门禁、P6/P7和Workflow UI整体验收继续保留未完成。
- 默认未启用可观测性时工作流访问日志的trace_id/span_id目前为null；尚未证明跨网关/服务/Worker关联，不能宣称关联日志门禁通过。

## 2026-10-03 后端剩余门禁复核

- 将过期 `TestWorkflowWorkerGatewayServiceToken` 替换为 `TestWorkflowWorkerGatewayAPIToken`，不再创建旧凭据或模拟登录。真实统一网关验证临时稳定 Worker 注册成功、User 冒充拒绝、缺少 heartbeat scope 拒绝、Token 撤销后注册及轮询均 401；不领取既有任务，只清理本测试创建的身份/Token。
- `TestAPITokenGateway`、`TestWorkerIdentityGateway` 和新 Worker Gateway 边界测试均通过；停用后的即时拒绝及后台 Token revoked 收敛已复验。仅证明这些认证边界，不勾选 trace_id 或跨企业 Dispatch 整体门禁。
- 历史恢复门禁已在 [历史迁移 Plan](media-ai-history-migration.md) 完成真实 API/Fixture Worker 续跑、受控媒体摄取与全量对账；这不等于生产 Provider 上传或 UI 预览验收。
- 当前 Worker API 路径只接受 Worker Internal JWT，不经过本地 Hash 验证；普通服务认证及 `media_auth` 仍存在旧 Service Token 实现，P7 尚未完成。Platform Token 仍需与其存储/调用方拆分，不能直接删除共用代码导致现有登录/OAuth 失效。
- Agent API Token 申请、身份权威/撤销以及代理入口仍未实现，P6 保持未完成；已有 Agent Session/内部服务凭据不作为该功能的证据。
- 按最新用户指令，全部 UI 工作等待新设计，本轮不实现；后端 trace_id 关联及剩余认证迁移继续保留在范围内。

### 默认关闭观测后端的请求关联

- 按 [请求关联规格](../spec/request-correlation.md) 增加不采样、不导出的 server/client 本地上下文及 W3C 传播；未启动观测后端，也未替换全局 TracerProvider。请求 ID 与 trace ID 保持区分。
- `hertz_infra` 独立模块 race 单测覆盖有效/非法父上下文、sampled 父级不触发采样、client span/header/错误及裸 HTTP propagator；模块不在 go.work use 中，门禁使用 `GOWORK=off go test -p 1 -race ./...`。
- 部署 Workflow/Materials 后，两者经真实统一网关返回 200，响应 `X-Trace-ID` 与各自落盘访问日志 `trace_id` 一致，span ID 各自不同且 trace_flags=00；5173 返回200。该验证不是异步 Worker/Outbox/Conductor 全链路 trace_id 门禁，P5 对应总项保持未完成。
- 只读 `TestRequestCorrelationGateway`、API Token 生命周期和 Worker Gateway 撤销门禁均通过；Workflow/Materials 全量 Go 测试及各服务 `make arch-check` 通过。根 Makefile 无 arch-check 目标，使用服务级已有目标，没有以根命令失败宣称通过。

### Agent 身份事实入口

- domain-modeling 对齐 Agent Type 与稳定身份：Actor 使用 Configuration 记录 ID；运行类型切换不改变 Actor，停止运行不隐式删除身份。Control 增加内部只读解析 IDL/生成客户端/路由/薄 handler，单次 join 只查询身份字段，回查 owner 当前部门授权。
- Service/handler race 测试、Control 全量 Go 测试及架构检查通过；真实 `TestAgentIdentityControl` 通过，核对 Workspace/owner/Organization/Team/Actor、未知 ID404、缺少/错误服务凭证401、非system403和无Provider Secret。公开 CLI `media_control workspace get` 实际执行通过；内部查询不新增危险公开CLI，网关未暴露此内部路径。
- Control 新镜像已部署且保持运行；此项只是 P6 的权威身份前置能力。Auth ActorValidator 尚未接入 Agent、Gateway 申请/代理以及停用/解绑撤销均未完成，未以身份查询上线冒充整项完成。

| 既有需求能力 | 目标上下文/Domain | 实现/流程 | 状态 |
|:---|:---|:---|:---|
| Agent 稳定身份/归属解析 | Control / agent_configuration | 内部只读投影、当前 Team 授权；无需写 Workflow | covered |
| Token Secret 生命周期 | Talos | API Token Issuance/Rotation/Revocation Adapter | external-owner（Talos） |
| Agent Token 私有签发/幂等 | Auth / api_token | 专用凭据入口、Control 权威校验、复用 API Token Issuance | covered |
| Agent 创建后申请/一次性回传 | Agent Gateway | Agent API Token Provisioning 的客户端编排 | missing |
| Agent 委托代理 | Agent Gateway | 受限内部身份代理，不能开放任意 URL | missing |
| Agent disabled/deleted/owner 解绑撤销 | Control + Auth | 权威生命周期事实、持久撤销意图/恢复 | missing |

### Auth Agent 校验接线

- `biz/shared/client/control` 使用生成客户端和固定内部 system 服务身份，传播 Trace Context、限制配置地址/ID和总期限，屏蔽上游错误正文；不缓存授权、不传用户凭据。取消/超时后的响应不能授权成功。
- `AgentActorValidator` 精确核对配置 ID、Actor、owner、企业、Workspace/Team和当前 owner 授权；Create/Verify/Rotate 共用此判定。Worker 权威保持独立，未知 Actor 拒绝；缺少 Control 配置时 Agent 不可用，不能退回本地 Hash。
- Client/Service race 测试覆盖跨 owner/企业、归属错配、失去授权、删除/未知状态、上游失败/超时/取消和敏感错误屏蔽；Auth 全量 Go 测试、架构检查通过，新镜像已部署。容器内只读查询验证 Auth→Control 网络/凭证配置，既有 User/Worker 生命周期、Agent Identity查询和请求关联的5项真实 HTTP门禁及CLI auth me通过。
- 没有通过真实 Agent Token 执行完整签发/代理/撤销：当前尚无新 Agent 签发入口，不用上述单测或容器探测替代该验收项。P6 其余项和能力矩阵中的 missing 保持未完成，UI未修改。

### Auth Agent 私有签发门禁

- 新增内部签发 IDL/生成模型/client/router/OpenAPI、薄 handler 与 AgentTokenProvisioning；专用 Gateway 凭据未配置即拒绝。身份字段只从 Control 派生，不接受请求体覆盖，scope 固定为 `agent.gateway`。
- Workflow/handler/Control client race 测试和 Auth 全量测试、架构检查通过。新 Auth 镜像已部署；`TestAgentTokenInternalProvisioning` 真实连接 Control/Talos，验证签发201、专用身份401、非owner403、未知配置404、重复409且不重放Secret、公开列表无Secret、撤销后inactive及普通API/Worker入口隔离。
- 既有 `TestAPITokenGateway`、`TestWorkflowWorkerGatewayAPIToken` 回归通过；公开 CLI 读取本次 Agent Token 元数据并重复撤销通过。仅清理测试自己创建的凭据，不启动 Agent Runtime，不领取任务。
- domain-modeling 同步重复请求、一次性响应和信任边界。上述覆盖 Auth 私有签发，不代表 Gateway 创建/代理、身份停用/解绑的持久撤销或 P6 整体完成；UI不在当前实施范围。
