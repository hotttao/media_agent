# 原生生成结果登记落地计划

依据 `docs/spec/generation-result-idempotency.md`。前置：用户已确认每轮独立产物语义，已有记录保留。

## P1 · 结果身份与回执核心

- [x] Workflow 稳定结果键纯函数及缺失/任务身份变化/同轮稳定测试；领域包 race 通过。函数只定义身份，不冒充引擎当前轮次选择已接线。
- [x] Materials 回执值对象、规范结果摘要与端口；领域包 race 通过短期 URL/时间/提议 Asset ID 不参与、对象/内容/来源变更摘要变化、未冻结媒体拒绝及 key/digest 校验。
- [x] Materials migration 006、Repository 键锁、事务要求、唯一性及降级保护；隔离 PostgreSQL 八路并发与回滚门禁通过。登记后的产物内容与回执受数据库不可变约束保护，允许软删除但保留回执，已有未登记资源不受新触发器限制。

## P2 · 内部接口与服务接线

- [x] IDL result_key 字段15，通过项目工具生成 model/client/router/OpenAPI；未手改生成文件。内部 handler 将登记冲突映射409，其他错误不泄露底层错误内容。
- [x] Materials 配置本地事务/回执仓储；来源验证后同键重放，首次对象验证在事务外，事务内 Asset/回执共享键锁。实际 Service + Repository + 隔离 PostgreSQL 八路并发、新 Service 实例重放、新轮独立 ID 与软删除不重建门禁通过；Storage/来源仍是受控端口，不冒充真实网络联调。
- [x] Workflow 使用当前轮唯一成功能力 Task ID；Materials 用稳定名称与完整键区分结果，同轮重复 Finalize 不换键，歧义/缺失/能力仍在执行时失败且不调用 Materials。单元测试覆盖旧轮、新成功任务、Finalize 重试与本地投影失败重放；同一 StepRun 经受控持久 Retry 边界后指向新 ID。该测试不冒充真实引擎 Retry 验收。
- [x] 保留旧无键兼容但原生引擎执行必须提供结果键；旧 Asset 不修改/删除，不启动全量历史迁移。

## P3 · 部署与验收

- [ ] 新旧结果不同 ID；同键并发/响应丢失/重启原 ID；不同摘要409，跨来源拒绝、软删除不重建。
- [x] 先部署兼容 Materials，再部署新 Workflow；仅新增一条四节点完整流程，包含重新生成、第二轮失败、Retry 与审批后继续。逐节点核对真实 Conductor 成功 Invoke Task、数据库回执与节点 Output ID 一致，旧资源仍可读。此轮未额外重复整条流程，不冒充成功后同节点再运行两次的真实跨轮资产测试。
- [x] Go HTTP 与独立 CLI 验收分别记录；实际 HTTP 四节点样本包含重新生成/失败/Retry/审批；另一个独立 CLI 单样本包含Submit/query/失败Step Retry/confirm直到四节点成功。mock 模型结果不冒充模型质量，隔离库测试不冒充运行接线。
- [ ] 全包、race、架构、gofmt、diff 门禁；最终 docs/wiki 示例使用最终协议与实际证据。

## 已验证证据（2026-10-05）

Materials 的 workspace 与 `GOWORK=off go test ./...` 均通过；Generated Asset service/domain 的 race、Materials 架构检查、改动文件 gofmt 与 `git diff --check` 通过。

Workflow 全包、Job service race 与架构检查通过。`TestExecutionProjectionAtomicPersistence` 在隔离 PostgreSQL 下通过（最新包耗时5.645s），补充验证登记前和登记期间接受的真实本地 Regenerate Review（RetryCount仍为0）会拦截旧轮结果写回；取消、Retry 快照与本地失败回滚门禁仍通过。审批同轮必须匹配已完成的审批 Task ID，并在 Job→Step 锁内再核对；此轮次保护没有外部调用。引擎/Materials端口仍受控，不冒充真实跨服务联调。

`GOWORK=off go test -tags=e2e ./e2e -run 'TestGeneration(ResultReceipt|RegistrationService)Persistence' -race -count=1 -v` 通过（包耗时13.205s）。仅启动并清理测试自己的隔离 PostgreSQL，不改运行中业务数据库。这证明本地登记协议，不替代跨服务丢响应或进程崩溃恢复测试。

## 实际测试环境部署与单样本（2026-10-05）

- Materials 镜像 `sha256:d447de915db4350bab49ac9bc0bc84860905df99d367902a81cb3fb2f416c6a7`；运行数据库 `schema_migrations=6,dirty=false`。先部署并经网关读取 Materials HTTP200 后才切换 Workflow。
- Workflow API 镜像 `sha256:f31d64047e80d8b79b1940399e23486b8f64c044ec04fa1d364875c4a8cf4346`。仅替换 API，不重启常驻 internal_worker 或基础设施。
- 启动客户端 fixture 前分页核对256条 Jobs，活跃0。仅新建 `job_a979a0f6-4f1c-4af9-a128-5a060b8d5581`，使用已发布四节点样本；客户端进程退出测试后停止，后端继续常驻。
- `TestWorkflowPersonalWorkerFixture` 实际通过11.80s：第一轮两候选→regenerate（纯文本反馈）→第二轮故障→显式Step Retry→用户选择索引1→四节点Succeeded；能力提供者结果mock，网关/API Token/审批/内部Renderer/Conductor/对象存储/数据库均真实。
- 只读对照真实 Conductor Tasks 与两库：Conductor COMPLETED；Materials 有4条结果回执；每条键均等于 `[executionID,stepRunID,当前轮唯一成功InvokeTaskID]` 的规范JSON SHA256，且 Asset ID 等于对应 StepRun Output ID。model_image为轮次2，其余为轮次1。
- `TestWorkflowCompletedOutputsGateway` 与 `TestWorkflowReviewReceiptsGateway` 实际通过，包耗时0.167s（明确非skip）：图片内容/视频Range可读、同键终态审批重放与相反决定409。独立编译CLI进程执行 `media_workflow job get` 返回Succeeded、4节点均有Output ID；这只是CLI查询门禁，不能替代本次CLI启动/审批/Retry全链路门禁。
- Vite5173页面HTTP200；未修改UI。旧资源只用于读取/签名解析fixture，不做历史回填。仍待真实网络丢响应/进程重启登记与完整CLI生命周期验收，尚未关闭整体开发目标。

## 补充登记恢复门禁（2026-10-06）

`GOWORK=off go test -tags=e2e ./e2e -run '^TestGenerationRegistration(HTTPReplayPersistence|RealProcessRestart)$' -race -count=1` 通过，包耗时10.367s，架构/gofmt/diff检查通过。

- `TestGenerationRegistrationHTTPReplayPersistence` 使用实际IDL、生成路由、内部Token中间件、handler、Service和隔离PostgreSQL；先验证未授权401不写库，随后提交成功而调用方丢弃返回ID，重建Service后以失效签名重放，返回原ID且不读Storage。不同内容409、不同Scope拒绝、新结果键新ID。Hertz `ut` 是进程内HTTP请求，不冒充真实TCP故障。
- `TestGenerationRegistrationRealProcessRestart` 启动真正独立子进程；数据库提交成功后 `os.Exit(77)`，不给父调用方返回产物ID。父进程只读回执，再启动新的子进程重放，必须返回原ID。新进程的任何Asset保存或Storage请求立即 `os.Exit(88)`，父测试要求退出0，避免将意外副作用当作恢复通过。最后两表均为1行。DSN只经私有子进程环境传递，不输出子进程日志或媒体载荷。
- 已有真实进程恢复证据，但Storage和来源仍为受控端口；真实跨服务TCP丢响应、完整CLI生命周期与整个开发目标的其他未完项目仍待验收，不用上述测试替代。

## TCP 协议与独立 CLI 补充（2026-10-06）

- `TestWorkflowRetryCLI` 实际通过10.67s（包10.710s），新建唯一CLI样本 `job_5c91fd58-e8f4-46ca-9e8f-aaa444cc0a83`。执行前完整分页核对活跃Jobs为0；仅临时启动客户端fixture，首个model_image Dispatch模拟失败。CLI独立进程完成dry-run、Submit、查询、Step Retry、选择两个候选中的索引1，最终4节点均Succeeded且有Output ID，model_image重试计数1。测试结束停止fixture，业务服务保持运行。最初发现跨日临时二进制不存在，预检查失败；重新只读核对Jobs仍257条且没有新增/活跃Job后才重新构建并执行，不因观察超时重提交。
- `TestGenerationRegistrationTCPLostResponse`（隔离Materials库/race）通过，包5.600s：受控TCP故障适配器调用实际Hertz生成路由/内部鉴权/handler/Service，成功提交后关闭真实客户端socket，不返回HTTP响应。检查首请求只执行1次，再由调用方显式同键重放，返回数据库原ID，不访问不可用Storage，两表仍各1行。这是实际TCP故障与持久化组合门禁，但适配器是测试设施，不冒充部署服务或网关断链。
- Workflow实际 `MaterialsClient` 补充TCP故障协议测试：必须透传相同ResultKey与内部Token，遇到EOF不自动重发，不将地址/原始传输错误交给上层，显式重放仍同键；缺失/空Asset ID或无效200响应均失败，上游错误body不回显。该单元测试的提供者为受控HTTP端口，不能替代Materials事务证明。
- 部署环境整体断网/杀死API后跨服务恢复、已完成节点的新轮资产真实生命周期以及其余主计划未完成项仍需验证；上述证据没有关闭整个目标，也没有修改UI或推进历史迁移。

本轮最终组合门禁：`GOWORK=off go test -tags=e2e ./e2e -run '^TestGeneration(ResultReceiptPersistence|Registration(ServicePersistence|HTTPReplayPersistence|RealProcessRestart|TCPLostResponse))$' -race -count=1` 通过（19.903s），涵盖5项隔离持久化/接口/进程/TCP测试。此前同组合命令的进程句柄失效，未取到终态，未计为通过；确认句柄不存在后重新执行仅隔离库测试，没有重复提交线上Job。Workflow全包、client/job race、两服务架构、gofmt和diff检查通过。CLI新样本的 `TestWorkflowCompletedOutputsGateway` 另实际通过（0.187s），验证新产物图片与视频Range可读取。本轮客户端错误收口修改尚未重部署API，运行样本使用前一轮已部署结果登记版本。
