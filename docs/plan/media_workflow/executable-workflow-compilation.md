# Executable Workflow 编译落地计划

## 审批 Retry Epoch 落地（2026-10-06）

- [x] migration021 nullable非负/不可变与安全降级，旧摘要兼容、新周期摘要/域验证。
- [x] 新审批冻结Job周期，锁内拒绝变更周期；原幂等回执不重绑定。隔离PostgreSQL `TestReviewRetryEpochPersistence` 与 `TestExecutionProjectionAtomicPersistence` 合并通过8.353s，包含旧周期原键重放、新周期同数字generation投影、陈旧观察拒绝、新记录摘要/不可变及非空降级保护。
- [x] 后台失效周期拒绝提交，投影忽略旧周期、不阻挡同数字generation新任务，未知旧周期不猜测。服务/domain/delivery race通过；新审批都带周期，旧null只按既有摘要核对，不回填未知事实。
- [x] 精确HUMAN Task ID投递：独立ApprovalTaskEngine端口取消后台引用名称二次查找，直接提交冻结ID。adapter单位门禁确认仅POST冻结Task ID、没有引用GET，空ID拒绝；真实Conductor丢响应/实际子进程重启两门禁合并通过13.999s。新周期并发Retry与旧Task状态的完整上游语义仍需额外验收；本轮已部署。
- [x] 域/服务/race、隔离PostgreSQL接受边界及新周期投影、部署后公开HTTP/CLI验收；UI/历史迁移不变。

部署与业务证据：2026-10-06 API镜像 `sha256:181f83a53a5748d5f048bb18abb3b1d12dc2f27bf732ace968468d1ea1ec3c57`，migration21/dirty=false，仅替换API。部署后旧Review仍12条null、0条新周期字段，证明未回填旧事实。起fixture前完整分页核对258 Jobs、活跃0；HTTP新样本 `job_d4c5a619-ba10-4e83-b284-950c47fd1d96` 通过9.67s，第一轮regenerate(epoch0)→第二轮失败→Step Retry→accept(epoch1)→四节点成功。只读对账真实Conductor精确HUMAN完成输出、两条Review applied及4条成功Invoke对应结果回执全部匹配。随后另做独立CLI样本 `job_72d2e8d0-b987-4f60-b530-1f5173ae612a`（7.58s），CLI Submit/query/失败Step Retry/confirm直到四节点成功，数据库确认审批epoch1/applied。每个样本只提交一次，测试后停止fixture，后端继续运行。媒体读取与审批重放分别通过0.110s/0.286s，覆盖新epoch回执及旧null回执；前端5173 HTTP200。能力结果mock，不能作为模型质量证据；未模拟运行服务整体断网，也不代替可靠Retry和并行DAG恢复。

## 最终交付 · Workflow / Conductor 实现说明（2026-10-05 补充）

- [ ] 完成功能收口与测试后，输出 `docs/wiki/workflow-conductor.md`；不以文档编写代替尚未完成的实现或验收。UI、历史数据迁移和 Agent 扩展仍不纳入当前开发范围。
- [ ] 说明整体抽象与责任边界：FlowGram 定义、业务 DSL / `slot_bindings`、Semantic Compiler、Executable Workflow IR、Conductor Definition、内部任务、客户端 Worker、业务状态投影；区分设计目标、已实现功能与实际验证结果。
- [ ] 总结设计取舍及实现细节，关联实际代码、持久化结构与交互协议：冻结版本、编译摘要、发布、启动、任务领取与回写、审批、重新生成、重试、取消、对账和故障恢复。说明为什么需要各项适配、替代方案及其成本和限制。
- [ ] 专门记录实测 Conductor 行为与处理方式，包括循环引用/迭代、终态 rerun 前提、按 seq 截断而非按业务依赖重跑、启动去重支持情况；注明引擎版本和测试证据，不能把请求字段或单测当作上游协议保证，未解决问题明确保留。
- [ ] 使用一个足够复杂、与最终实现相符的示例，贯穿定义→编译→发布→执行：并行分支与汇合、多图片选择、单图片强制审批、反馈重新生成、节点失败及用户重试、最终视频产物。给出关键 DSL / IR / Conductor 片段及执行时间线，解释各层 ID、幂等键作用域、事务边界、租约 fencing 和审批轮次。
- [ ] 在示例中解释重复请求、提交成功但响应丢失、进程重启、过期 Worker 回写及取消竞争如何处理；说明如何避免重复执行已完成旁支。涉及尚未支持的组合不得编造成功流程，应明确边界并引用对应测试或待办。
- [ ] 最终核对 wiki 与 DDD / spec / 实际代码、HTTP / CLI / Conductor / PostgreSQL 验收一致，区分 mock 模型返回与真实模型调用；示例和证据不包含凭据、签名 URL 或敏感上游错误。

## 当前功能收口（2026-10-04）

- [x] 普通 User Submit 校验目标 Workspace、当前执行权限并冻结企业；Worker Poll 拒绝跨企业或无企业 Job，覆盖单测。
- [x] 部署上述改动并以一条新任务验证归属、四节点执行及输出读取；`job_91f4a339-6b5c-42e6-9656-4cc8aee01790` 四节点成功，3 PNG + 1 MP4 实际 Range 206 读取，输入无签名且企业/Workspace 正确。
- [ ] 普通新任务冻结稳定媒体引用，在排队、审批和重试后交付时重签。
  - [x] Submit 媒体输入验证并冻结；Personal/Managed Dispatch 快照与交付响应分离，租约重取时重签，覆盖对象不可用和企业隔离。
  - [x] 覆盖模板内已知媒体引用、受控上游产物链接及带输入覆盖的 Retry；重试使用冻结的 IR/Template Schema，校验失败不重跑。
  - [ ] 部署并完成运行环境联调，不以单测代替审批等待后的真实四节点执行证据。
  - [x] 新建四节点单样本完整执行与输出读取通过；专门的长审批等待、重新生成及带覆盖重试运行测试仍待验收。
  - [x] 修复预期单候选但实际多候选的审批路径；独立真实 Conductor 门禁覆盖单候选自动、多候选 HUMAN、单候选 always 及选定候选收口，不替代 User/Worker Gateway 验收。
  - [ ] 修复等待审批时的重新生成：当前直接调用 Conductor rerun 不满足其“执行必须终态”的契约；需以可恢复的审批循环实现，不能粗暴终止整个并行 Workflow。
    - [x] 编译循环及安全引用命名，Finalize 留在循环外；新发布生效，不覆盖旧 Compilation。
    - [x] 审批 API 完成当前轮精确 HUMAN；对账忽略旧轮状态及候选，保留并行分支。旧无循环实例明确拒绝重新生成，仍允许确认。
    - [ ] 持久化审批决定/Outbox、反馈及幂等命令，补齐故障恢复验收。
      - [x] 审批记录迁移和 Repository：隔离 PostgreSQL 通过事务回滚、8 并发同键重放、同轮互斥、跨轮隔离、claim 过期回收、旧 claim fencing、退避及保留审计的迁移回滚；迁移 016 已随测试服务启动部署。
      - [x] `review_delivery` 后台流程及真实 Conductor 响应丢失门禁：第一次实际提交成功后丢弃响应，重建流程查询精确 HUMAN 后补记 applied，实际提交次数保持 1；错误只保存稳定码。该门禁不代替用户接口或后台进程装配。
      - [ ] 接入用户命令、后台投递、已提交响应丢失恢复及 pending Review 投影保护。
        - [x] 用户命令与后台已编码装配；显式键优先重放，回执与投递状态分离；投影采用聚合事务锁，单测覆盖权限、过期候选、数组位置、终态重放及无外部提交。
        - [x] 测试服务持久审批单样本 `job_a3820e40-3f24-4ddd-91f8-111488dd9c63` 完成 User Gateway→Worker→重新生成同键重放→确认同键重放→四节点成功；数据库仅两条 Review，第一轮 regenerate、第二轮 accept 均 applied；3 PNG + 1 MP4 Range 206 实读通过。显式 fixture、非真实模型、无历史迁移，测试 Worker 已停止。
        - [x] 隔离 PostgreSQL 用户命令测试覆盖旧 waiting HUMAN 投影不会重开已提交审批、终态显式重放及同键反向决定冲突。
        - [x] `TestWorkflowReviewReceiptsGateway` 在新版服务经真实 HTTP 验证已完成任务同键重放返回原 applied 回执、202，反向决定 409；不提交新 Job，不重新执行 Worker。
        - [x] 待投递决定的实际进程重启协议验收（2026-10-05）：`TestReviewDeliveryRealProcessRestart` 使用隔离 PostgreSQL 和真实 Conductor，独立子进程领取后直接退出、实际提交后落盘确认前直接退出两场景通过。另一独立进程回收过期 claim、恢复原决定为 applied；已提交场景禁止再次 CompleteTask。仅调整隔离夹具的 lease 时间，不修改用户数据；此门禁不代替运行中 API 容器重启及用户 HTTP 接线验收。
        - [x] 纯 Renderer 普通文本反馈与完整摘要、编译相邻轮次参数、用户命令能力门控及幂等冻结；单测及相关包 race 通过。
        - [x] 新定义真实 Conductor 五场景通过，验证首轮无反馈、第二轮实际提示词包含原文、重新生成后自动分支不复用旧决定；已部署新 API/内部 Worker，不原位修改既有 Compilation。
        - [x] 新四节点反馈样本 `job_4aa9476e-f3d0-4779-87de-9ee5e537676a` / `wf_29f16595-610a-4570-8704-69405f52c668@1` 经真实网关执行完成。反馈带 `{{...}}` 字面文本仍原样进入下一轮提示词；同键重放与最终确认通过，测试 Worker 已停。使用已有媒体 mock、不调用真实模型、无历史迁移。
        - [x] 上述反馈样本数据库恰两条 Review，冻结反馈与 output feedback 一致且均 applied；终态 HTTP 回执/冲突测试通过，3 PNG + 1 MP4 Range 206 读取通过。
    - [x] 经用户 API、Personal Worker Gateway 执行“多候选→重新生成→确认→最终产物”：样本 `job_ff564352-7bac-46c8-8b0b-8c4828a94dcf` / `wf_6b810add-e7e7-4f03-bc74-a2cdbe398bbc@1`。新发布 when_multiple_candidates、预期单候选，但实际两张受控图片进入 HUMAN；重新生成后第二轮仍两候选，用户选择数组位置 1，最终对象路径与第二张匹配。两轮不同 Task ID/Dispatch；其他节点各执行一次；两条 Review 均 applied，第二轮 selected_index=1。四产物 Range 206、终态 HTTP 原回执/相反决定409通过；测试 Worker 已停，显式 mock、无真实模型/历史迁移。
    - [x] 新增 `media_workflow job get` 与 `step-run get/confirm/regenerate` 独立 CLI，稳定键必填，安全输出状态摘要。相关包 race 通过；真实网关跨进程查询、确认重放、带反馈重新生成重放及相反决定拒绝通过，不提交或重新执行 Job。
    - [x] 独立四节点 `always` 单候选审批样本经实际 User API→Personal Worker Gateway→重新生成→确认→成功，Job `job_6bb88b6d-c206-493c-bf64-6f62917eb97b`，Workflow `wf_9816682a-92b3-49b5-8640-5de19cd36f71@1`。模型图两轮不同 Dispatch，其余节点各执行一次；3 个 PNG、1 个 MP4 的 Range 206 实读通过。此样本使用显式 fixture，不调用真实模型、不迁移历史；测试 Worker 已停，服务常驻。
    - [x] 新编译结果真实 Conductor 五场景通过：单候选自动、实际多候选、单候选 always、重新生成后审批、重新生成后自动。第二轮自动分支不得复用第一轮 HUMAN 的 regenerate 决定。
  - [x] 独立真实 Conductor 审批循环协议门禁：重新生成创建第二轮独立 Task ID、审批输入与最终引用使用第二轮候选，旁路 HUMAN 的 Task ID 和等待状态保持不变。循环内引用不得包含 Conductor 的 `__` 迭代分隔符；该测试尚不证明业务编译器、审批 API 或 Worker Gateway 已接入循环。
- [ ] 汇总新任务执行、审批、重新生成、重试和 Materials 接口验收证据，再处理历史数据；UI 和 Agent 扩展暂不执行。
  - [x] Step Retry 选择最新轮 Renderer，拒绝非失败执行、超过重试上限、缺失或歧义的当前轮任务；单元、race、全包测试与架构检查通过，API 镜像已部署（2026-10-05）。真实 Conductor `retry regenerated capability` 证明第二轮失败后重跑第二轮 Renderer 保留用户反馈、产生新 Capability Task ID。
  - [x] 经 User API、Personal Worker Gateway 的串行失败节点重试单样本通过（2026-10-05）：`job_fe0abe36-32e1-4783-b59d-2a05618c8e42`，首轮多候选→带反馈重新生成→第二轮 Worker Fail→Job/StepRun FAILED→用户 Step Retry→重新审批选数组位置 1→四节点完成。模型节点 retry_count=1、3 个不同 Conductor Task/Dispatch，其他能力各一次；第一轮 regenerate、第二轮 accept 两条 Review 均 applied，反馈保留。产物 3 PNG + 1 MP4 Range 206 与终态回执/冲突测试通过；fixture Worker 已停止。独立 CLI 重试成功链路尚未验收，不把 HTTP 门禁冒充 CLI。
  - [ ] 并行分支重试不重复已完成旁支的设计与证明；Conductor rerun 的 seq 截断语义不能用串行测试冒充并行证明。Submit/Retry 的跨数据库与引擎可靠协调仍未完成。
    - [x] `TestConductorParallelRerunTruncatesCompletedSibling` 真实引擎协议测试确认已完成旁支被截断；新增直接重跑隔离检查，单测/race 覆盖已完成后缀、失败/等待前缀、缺失 seq、未知能力任务，在任何变更前拒绝。此保护不等于并行恢复已实现。
    - [ ] 冻结 DAG 重试集合、可靠重试意图与旁支结果恢复；保留目标最新轮反馈、不再次交付已完成能力；补齐并行 User/Worker Gateway 和故障恢复验收。
    - [x] 普通引擎 Start 响应丢失时，只查询 correlation 找回唯一已提交实例，不自动重发；真实 `TestConductorStartCommittedResponseLossRecovery` 通过，上游实际启动后注入 503，恢复同一 ID 且只有一次 Start。单测/race 覆盖查不到和多个匹配时安全拒绝，无重复 Start。
    - [ ] 启动幂等上游协议门禁：运行镜像 `conductoross/conductor:3.32.4` 的 `TestConductorStartIdempotencyContract` 失败，相同 `RETURN_EXISTING` 键仍创建不同实例。测试仅创建独立 HUMAN 技术流程并终止，不修改用户执行。不能据请求字段存在假定服务支持幂等，也不能据 correlation 暂未查到自动重发；需确定并验证兼容的引擎启动方案，再实现普通 Submit 持久 Outbox、进程崩溃与丢失响应全链路恢复。现有历史 Resume 的字段级单测不提供该上游保证，本轮不推进历史恢复。
  - [x] 新 API 部署后回归 `TestWorkflowReviewReceiptsGateway` 和 `TestWorkflowCompletedOutputsGateway`：原终态回执重放、相反决定拒绝、3 PNG + 1 MP4 Range 206 通过；网关任务查询和 Vite 5173 HTTP 200。不是新的执行样本或 UI 交互验收。
  - [x] 独立 CLI 失败重试验收（2026-10-05）：新增 `step-run retry` 和 retry_count 安全摘要，参数/200匹配/错误隐私/单次请求单测及 race 通过。`TestWorkflowRetryCLI` 跨进程执行 dry-run、一次 Submit、Job Get、Step Retry、Confirm，样本 `job_371f098d-95f5-4714-b9b2-ad920bb6a3f8` 四节点成功。数据库模型节点 retry_count=1，两个不同 Task/Dispatch，其余能力各一次，一条第一轮 accept Review applied、selected_index=1；显式 fixture Worker 已停止。补齐前述“独立 CLI 重试尚未验收”，不证明持久重试意图或并行恢复。
    - [x] 该 CLI 样本的 3 PNG + 1 MP4 通过真实存储 Range 206 只读读取；没有用仅存在资源 ID 替代媒体可用性验收。

## P1 · 领域类型与纯函数

### 原生执行产物与终态投影收口（2026-10-05）

- [x] Finalize 不再忽略 Output/Status 保存失败，拒绝空产物 ID；外部幂等 Materials Upsert 在事务外，本地 Output + SUCCEEDED 在调用方事务内，以 Job→Step 锁顺序与审批/初次启动取消协调。重读终态后拒绝复活；Conductor 模式不运行旧 advanceDAG/checkJobCompletion。
- [x] 引擎 COMPLETED 不再单独证明业务成功：所有持久节点必须 SUCCEEDED 且有 OutputAssetID；候选/提示词投影写入和反序列化错误向上传播，失败/暂停终态投影重新读取 Job，不覆盖已有终态。
- [x] 服务错误路径/race 测试通过。`TestExecutionProjectionAtomicPersistence` 在隔离 PostgreSQL/race 通过（31.27s）：状态实际写入后注入错误，Output/Status 一并回滚；重建读取重试保存后成功，终态读取不重复保存，外部 Upsert 期间取消不复活。Engine 和 Materials 为控制端口，不冒充真实上游、Materials 去重或 HTTP 证明。首次夹具漏绑定引擎 ID 已修正，重新执行通过。
- [x] 非 Finalize 阶段补齐投影锁：Render/Invoke/失败 Finalize 在 Job→Step 锁内重读终态，Invoke 候选重置与状态一起保存；已完成且有产物的旁支不被旧 Invoke/审批快照改回等待。服务/race 回归取消后晚到 render/invoke/finalize 和旁支回退；扩展 `TestExecutionProjectionAtomicPersistence` 隔离 PostgreSQL/race 通过（38.57s），证明 Invoke 更新后故障的候选回滚，以及 render/invoke/finalize/approval 读取旧快照后取消不再写入。本测试使用控制 Engine，不证明网络乱序或 Retry epoch 隔离。
- [x] API 镜像 `sha256:b98a51be39e4e9b53f3b3fdaa88813e5ecc2d6304ccf576ec4717efdb3c66106` 部署且 running；真实网关 `TestWorkflowReviewReceiptsGateway` / `TestWorkflowCompletedOutputsGateway` 通过（2.370s），独立当前源码 CLI 读取已有成功 Job 通过，Vite 5173 HTTP 200。只读回归原样本，不把它当作新四节点执行、并行执行或新投影故障的在线证明；客户端 Worker 未启动。
- [ ] 部署并完成新四节点网关执行/独立 CLI 验收；并发过期引擎快照与 Retry 轮次隔离、素材落库响应丢失的真实恢复证明仍需补齐。
  - [x] 已持久化 Retry 边界的快照 fencing：在 Engine.Get 前冻结 Job/实例/RetryCount，投影锁内重读比较；Finalize 外部保存前与本地保存后双检查；过期 Get/List 返回当前记录，不继续投影。服务/race 覆盖 count 和实例变化、外部保存期间 Retry；隔离 PostgreSQL/race `TestExecutionProjectionAtomicPersistence` 扩展五阶段旧快照读取后实际 AppendRetryHistory 通过（43.23s），不改运行数据。全包/架构通过，尚未部署本轮修改。
  - [ ] 上述校验不覆盖 rerun 已提交、本地 RetryCount 尚未增加的窗口；仍需可靠 Retry 意图、原子本地接受及不确定上游操作对账。重新生成 generation 改变但 RetryCount 不变的跨查询乱序也未完成。
    - [x] 2026-10-06扩展持久Review轮次校验到Render/Invoke/失败Finalize/审批等待以及Job状态投影，Job→Step锁或Job锁内核对；旧快照不会覆盖已经接受的regenerate/accept。服务race覆盖5阶段×2决定，以及上一轮regenerate不阻挡真实新数字轮次；隔离PostgreSQL `TestExecutionProjectionAtomicPersistence` 最新通过5.563s，真实AcceptReview在Engine快照读后提交、RetryCount仍0，提示词/候选/节点/Job状态保持接受事实。全包/架构通过，未部署。
    - [x] Review与Job Retry epoch绑定、摘要/锁内校验/旧周期投影隔离及精确HUMAN Task ID投递已部署并通过HTTP/CLI四节点门禁，详见本文审批Retry Epoch阶段。可靠Retry意图及引擎提交到本地计数提交窗口仍未解决，不以周期保护代替全部Retry恢复。
  - [ ] 产物轮次幂等：核对 `media_materials.GeneratedAsset.Upsert` 当前按 StepRunID 查到即返回旧产物；整单 Retry 重跑已完成 Step 时会复用旧资源。需以冻结的执行/生成轮次区别新产物与同轮重放，保留旧产物审计，并验证新旧轮结果不串用；不以本地 snapshot fencing 冒充跨服务解决。
    - [x] 用户采纳每轮独立产物方案；DDD 已取消默认覆盖语义，落实规格和阶段计划见 `docs/spec/generation-result-idempotency.md` / `docs/plan/generation-result-idempotency.md`。同轮登记返回原 ID，新轮独立 ID，旧记录不覆盖或删除。
    - [x] 2026-10-05 已部署Materials migration006/内部ResultKey协议和Workflow成功Invoke任务接线；实际HTTP样本 `job_a979a0f6-4f1c-4af9-a128-5a060b8d5581` 完成regenerate→故障→Retry→审批→四节点完成，真实回执与Conductor成功任务/节点Output ID逐一匹配。2026-10-06独立CLI样本 `job_5c91fd58-e8f4-46ca-9e8f-aaa444cc0a83` 完成Submit/query/失败Step Retry/confirm到四节点成功。隔离PostgreSQL门禁覆盖并发、回滚、新旧ID、冲突、软删除、进程提交后退出/新进程重放与受控TCP丢响应；来源/Storage仍为受控端口，不冒充部署环境整体断网恢复。
    - [x] Finalize登记前与Job→Step锁内核对持久Review轮次及已完成审批Task，阻止RetryCount不变的regenerate决定之后旧Finalize重挂节点；隔离PostgreSQL真实AcceptReview、服务race门禁通过。已上线首版本，不代表所有render/invoke阶段的generation乱序投影都已解决。

### 原生 Submit 可靠启动补齐

- [x] 公开提交回执持久层（2026-10-05）：领域校验、migration 019、完整作用域隔离与事务锁；`TestSubmissionAcceptancePersistence` 隔离 PostgreSQL/race 通过八路同键接受返回同一 Job，Job/StepRun/启动/回执各只有一条；事务回滚无部分聚合，摘要冲突拒绝、终态回执重放、User/企业/Workspace 隔离、无事务拒绝及降级审计保护。域单测/race、全包和架构检查通过。此为 Repository 门禁，不提供 HTTP/CLI 幂等或部署证据，运行库仍为 18。
- [ ] 服务/IDL/CLI 接线：授权后优先原回执重放、稳定媒体归一化、同键不同语义409；部署并完成网关与独立 CLI 并发/终态重放，不能以仓储测试替代接口证明。
  - [x] IDL SubmitJobReq 字段7、Handler 409、安全 CLI 稳定键与错误摘要；按项目目标重新生成 model/client/router/OpenAPI，不手改生成产物。CLI race 覆盖键传递、非法键、单次 POST、409 区分、503 保留原键指导和错误体隐私。API 镜像 `sha256:5551f80282c8da1b70f2d3e5fe2eca8604d267463931d97b2dfdf16f79e8da18` 已部署，数据库 version=19/dirty=false。
  - [x] 真实网关 `TestWorkflowSubmitIdempotencyGateway` 通过：新取消专用样本 `job_445cce2d-ce7f-48dc-8fc9-5904eb1ab9d4` 八个同键并发请求返回原 Job，后台启动后取消，无效新签名终态重放返回原 cancelled，改请求版本409；数据库一条提交回执、一条初次启动，清理 completed。此处 completed 指清理完成，不是业务 Workflow 成功。
  - [x] 独立 CLI `TestWorkflowSubmitIdempotencyCLI` 通过：三个独立进程复用同键，样本 `job_c6c77699-6843-461e-97f5-2b1e614ba897` 接受、重放、取消后重放均返回同一 Job；数据库回执/启动清理已收口。CLI 二次组装仍读取当前资源，不冒充“发布版已删除时离线重放”或 CLI 并发证明。两样本各4个 StepRun，未启动客户端 Worker/模型，不是新增完整生成流程。
  - [x] 服务层 SubmitCommand 稳定键、只读媒体归一化摘要、授权后原回执优先重放，以及首次接受事务内锁键二次查询/保存回执（2026-10-05）。`TestSubmitReceiptReplayPrecedesPublicationAndMediaRead`、`TestSubmitConcurrentWinnerRecheckedBeforeAnyAggregateSave`、`TestSubmitReceiptCannotExposeForeignOrCorruptAcceptedJob` 和服务包 race 通过：签名变化重放、终态原状态、发布版不可用、无外部读取/同步 Start、内容/请求版本冲突、权限撤销先于查询、并发赢家避免多余聚合、回执指向异租户或非 native Job 拒绝。main 源码已配置仓储，尚未部署；IDL/handler/CLI 未暴露稳定键，不视作公开接口已经具备幂等。

- [x] migration 017、ExecutionStart 冻结摘要与 Job 同事务 Repository；`TestExecutionStartOutboxPersistence` 隔离 PostgreSQL/race 通过回滚、唯一初次启动、JSON/状态/lease SQL 约束、八路并发领取、过期租约、旧 token、发送标记不回 queued、错误码隐私、摘要不符时绑定回滚、正确绑定原子性、取消不复活与降级保护。域摘要/参数单测、全包测试及架构检查通过。仅隔离数据库应用迁移，尚未接入 Submit 或部署运行数据库。
- [ ] 接入 Submit 与后台 Start Delivery；取消/不确定实例终止、只查询恢复、参数与版本冻结；不得重发不明确 Start。
  - [x] `start_delivery.Flow` 投递核心及只读 `ExecutionStartEngine` 对账接口（2026-10-05）：queued 必须先成功持久发送标记再 POST；uncertain 只查询 correlation，查不到或查询失败均不重发；绑定失败保留对账路径。单测/race 验证冻结参数、取消不启动/不复活、标记失败不 POST、摘要变化拒绝及上游错误隐私。尚未装配在线 Submit/后台循环，也未完成取消后上游清理和上游启动去重，不视作可靠启动验收。
  - [x] Submit 增加 `ConfigureStarts` 持久接受路径：保存前解析/校验冻结启动参数，Job + StepRun + ExecutionStart 在一个调用方事务内保存；返回 pending 聚合而非在事务提交后进行可能失败的读取，不在请求中 Start。服务单测/race 验证共享事务上下文、稳定媒体输入、冻结版本、校验先于保存、配置缺失拒绝、持久化失败不暴露内部错误以及无上游启动。测试中的事务探针不证明数据库回滚，真实回滚证据仍为隔离 Repository 门禁；在线 main 尚未启用此路径，取消协调与公开 Submit 幂等未完成。
  - [x] 启动取消持久协调（2026-10-05）：migration 018 扩展 cancelled/cancel_pending；`CancelInitialStart` 原子取消 Job 与初始命令，`FinishCancelledStart` 保留实例审计并以摘要/claim/租约隔离晚到结果。隔离 PostgreSQL/race `TestExecutionStartOutboxPersistence` 通过未发送取消、重复取消、不再领取、uncertain 晚到绑定拒绝、旧 claim 拒绝、已绑定实例清理及降级审计保护。`start_delivery` 支持只查询取消的未知实例、确认引擎终态后收口；单测/race 覆盖 Terminate 响应丢失后只读恢复。Cancel 用例已编码接入配置端口；运行库尚未部署 018、在线 main 尚未启用、真实引擎取消竞争仍待验证，不能据这些测试宣称运行环境已完成。
  - [x] 真实引擎 + 隔离 PostgreSQL/race `TestExecutionStartDeliveryCancellationRecovery` 四场景通过（2026-10-05）：发送前取消零 Start；Start 真正提交后注入响应丢失并阻断即时查询，重建协调器恢复原实例且只启动一次；未知启动随后取消；Terminate 真正提交后响应丢失，重建协调器先读终态、不重复终止，最终原实例 TERMINATED、数据库审计 cancelled、Job 不复活。故障来自测试代理，不修改运行用户记录；此门禁不代替独立进程崩溃验收。
  - [x] main 配置 Start Repository/后台有界投递，Conductor Submit 移除同步回退；缺配置拒绝保存。部署 API 镜像 `sha256:cd49468c998a2af754fb7e9747aec8cabccf7d51c66898e7dff62a935533a9a4`，运行库 version=18/dirty=false。公开网关新取消专用样本 `job_4a932979-2d31-4a95-b9f7-2ae4c93b4cc6` 经 Submit→后台 running→Cancel→重复 Cancel，数据库 Job/启动命令均 cancelled，原引擎实例 TERMINATED；无客户端 Worker/模型调用。补齐上述“尚未部署/接线”项，但不证明完整四节点新执行、公开 Submit 幂等或真实进程重启。
  - [x] 部署后完成任务产物 Range 206 与原审批回执/相反决定拒绝回归通过；使用 CLI 样本检查产物、使用原 fixture 命令键样本检查回执，不把不同命令键当成重放。Vite 5173 HTTP 200，未修改 UI。
- [ ] 公开 Submit 幂等、真实网关单流程、丢响应和进程重启门禁；上游启动去重缺口仍保留为待解决项。
  - [x] 独立进程协议门禁 `TestExecutionStartRealProcessRestart`（2026-10-05）真实 Conductor + 隔离 PostgreSQL/race 通过：领取后 os.Exit、Start 已提交但 FinishStart 前 os.Exit、Terminate 已提交但清理审计前 os.Exit。另一独立进程回收隔离租约，绑定原实例/确认终止；恢复已提交的启动或终止时，任何再次调用 Start/Terminate 都会让子进程异常退出，测试不得以吞掉重复调用错误冒充通过。核对唯一 correlation、Job/启动记录状态和领取次数；未操作运行用户数据或在线 API 容器。
  - [x] 同一门禁单独证明 mark-before-send 崩溃的安全边界：新进程只查询且不会发送，Job pending/命令 uncertain 无实例，释放自己的 claim 并保留未解决事实。该场景不是自动恢复成功；不得关闭上游幂等缺口或把“保持不重复”当作可靠启动已全部完成。最终修正版完整通过 60.940s，初版终止场景漏算首次启动领取的断言已纠正。

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
  - [x] `CheckWorkflowDefinition` / `EnsureWorkflow` 精确 name/version 读取、编译摘要和非默认扩展检查；已存在一致则只读重放，已知冲突不覆盖，PUT 后必须读回，提交响应丢失只查询、不盲重发。Publication 在 Compilation 保存和 Task Definition 写入前预检，registered 前复核；稳定错误码不落原始上游错误。Adapter 单测/race 覆盖冲突、额外执行字段、查询故障、实际提交/未提交的丢失响应、注册后改变。
  - [x] `TestConductorRegistrationFrozenDefinition` 真实引擎协议门禁通过：唯一技术 HUMAN 定义的 PUT 已实际提交后注入 503，通过精确 metadata 读回恢复；同内容重放和不同内容拒绝都不再 PUT，原定义仍匹配。测试不启动实例、不注册共享任务定义、不操作运行用户 Compilation，清理仅本次定义。
  - [x] 本地Compilation Save改为首次插入、同冻结内容重放、不同内容冲突；JSONB相等而非易变transport字节；不重置生命周期。MarkRegistered保持首次时间，已registered后晚到失败不回退，缺失ID返回NotFound。migration020冻结IR/Definition/身份/hash/创建时间，并保护registered终态。隔离PostgreSQL并发/回滚/原记录保留/不同版本与直接DB修改拒绝验收见 `TestCompilationFrozenPersistence`；最新通过4.803s。2026-10-06已部署，仅运行回归不作为用户新发布API验收证明。
  - [x] TaskDefinition同名契约预检、只POST缺失子集、提交后逐名GET确认；去掉失败后PUT覆盖。请求重复名称矛盾在网络前拒绝，整批预检冲突前不写；默认值依据锁定源码与真实Conductor。单位/race门禁覆盖精确重放/不同retry/模板扩展/监听关闭/显式零值/查询503/丢响应/未提交/写后冲突；真实Conductor独立测试名提交后代理503、GET恢复、冲突拒绝及原定义保留通过。现有6个样本TaskDefinition只读兼容门禁通过，未写生产任务定义。
  - [ ] 可靠发布意图/后台重试未补齐；上游没有CAS的并发外部作者窗口未解决。本地Compilation和TaskDefinition预检不代替这些事项。

发布保护部署证据（2026-10-06）：Workflow API镜像 `sha256:04060315226c3820bef7c65ee4e067fa2d752fe6f254e22a591ea2aaab4a6d61`；运行库migration20、dirty=false；仅替换API，未重启internal_worker或基础设施。真实TaskDefinition技术样本与现有6任务只读门禁通过（包1.252s），测试结束仅清理自己的唯一技术任务名，没有创建实例。部署后 `TestWorkflowCompletedOutputsGateway`/`TestWorkflowReviewReceiptsGateway` 真实回归通过（0.180s，非skip），读取CLI成功样本并重放既有HTTP样本审批。Workflow全包/conductor race/架构/gofmt/diff检查通过。此部署也包含前轮MaterialsClient错误收口修改；不代表Publication Outbox或新发布HTTP/CLI已验收。
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
- [x] 只读清点旧 Workflow/WorkflowTemplate/VideoTask/Video，导出 0600 初始源清单；确认 153 条 VideoTask 的单任务来源均标为 manual_upload。
- [x] 沿 ModelImage→StyleImage→FirstFrame→Video→VideoPush 关联重建旧视频谱系与异常清单，不能以 `VideoTask.source` 或 `workflows` 表直接判定业务流程。
- [ ] 按 WorkflowTemplate 的动作集合和 VideoPush 的多视频引用归并旧流程实例，保留复用中间结果与未归属的独立产物。
- [ ] 为历史执行谱系定义幂等的导入 API、对象迁移和旧 ID 对照；无下游 Video 的完整图片链须导入为待继续执行，具体跨服务步骤见 [media_ai 历史迁移计划](../media-ai-history-migration.md)；再逐条导入、核对总数与媒体可访问性。

## P8 · 人工审批闭环

- [x] IR v2 冻结候选数量与是否需要审批。
- [x] Backend Compiler 在需要审批的 Capability 后插入 HUMAN task。
- [x] 对账 HUMAN input 为 StepRun candidates/need_confirm，并从 UI 确认唯一候选。
- [x] 确认后完成 HUMAN task，Finalize 登记 Generated Asset 并继续 DAG。
- [ ] 重新生成闭环：旧实现直接 rerun 等待审批中的 Capability，已确认不满足真实 Conductor 契约；须接入审批循环并通过业务 API 验收。
- [x] Cancel/Retry 同步控制 Conductor execution；带覆盖重试运行验收仍见当前功能优先项。

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

> P9 记录已完成的旧 Service Token/User Domain 基线。API Token、Delegated Actor、Capability Execution Target 与 Managed Worker 的替换计划以 [API Token 与 Worker Execution Target 落地计划](../api-token-worker-execution.md) 为准。
