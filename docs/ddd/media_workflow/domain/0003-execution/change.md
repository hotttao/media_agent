# execution domain 变更记录

## 2026-10-07 · Worker结果改为直接原生确认

用户确认撤出正常Inbox写入与后台发送，精确Task确认后仅记账Dispatch；旧表存档、未交付旧记录阻止切换。Personal通用框架保存待确认结果并恢复交付，Provider幂等仍必要。初次启动/发布/审批Outbox不在删除范围。

## 2026-10-07 · Worker失败分类为原生任务结果

失败回执允许明确transient/permanent，分类参与结果去重。结果接收仍只原子保存Inbox和Dispatch接收状态，不更改Job/Step；重试权威为Conductor。旧空分类与显式永久失败规范化一致，保留已接收旧请求重放，不迁移用户历史结果。

## 2026-10-07 · 产物登记由原生Finalize驱动

移除查询触发登记的设计：真实Asset在内部Finalize任务中登记后回写Conductor，Job/Step仅为可重建投影。结果键绑定成功invoke而非Finalize技术尝试，临时故障交原生自动retry；无效契约明确永久失败。旧冻结定义不改写，历史已有引用不删除。

## 2026-10-07 · 用户确认节点入口采用原生retry

用户明确无需单节点局部重跑语义：Step Retry代理所属实例，与Job Retry一致；撤出影响集合、renderer定位及本地节点重置。原生retry不增加旧整次rerun周期，缓存failed不再阻止状态刷新或已恢复Task交付；技术attempt来自Conductor。输入覆盖保留明确的旧rerun兼容分支，其他边界整改仍需验收。

## 2026-10-07 · 执行权威收敛为Conductor

用户明确采用Conductor以简化执行语义：取消未部署execution_retries控制聚合/表，Job/StepRun为业务关联和查询投影，不再按本地重试周期驱动执行。既有审批、资源、Worker幂等业务边界保留；运行数据未删除。

## 2026-10-07 · 可比较的重试引擎证据

- 仅有基线digest不足以进程恢复，增加摘要化Task快照和完整Definition/输入/路由摘要，不保存签名URL或Task正文。
- 新输入与任务完整重置同时成立才持久确认；原始精确任务、顺序及前置快照防止半完成和歧义匹配。确认与业务应用分开。

## 2026-10-07 · 重试投递租约

- 数据库时钟、独立随机token和两分钟租约隔离后台进程；标发送在Job锁后复验冻结作用域/周期，并仅queued可转uncertain。
- 释放只保存安全码与十秒退避，旧token/过期租约拒绝；投递次数不等于业务重跑次数。

## 2026-10-05 · 生成结果身份独立于业务节点

用户确认每轮独立 Generated Asset：StepRun 不换身份，当前产物引用可随新轮接受更新；能力结果身份在 Finalize 技术重试和网络重放间不变。取消、Retry 和旧快照不得重关联上一轮结果；旧 Asset 不覆盖、不删除。

## 2026-10-05 · 公开提交意图去重

增加 User/企业/Workspace 隔离的不可变提交回执，同键参数不变返回原 Job，变化冲突。与首次启动同事务接受；不通过输入相同推断用户意图，不复用 Retry 身份。

## 2026-10-05 · 原生初次启动事实

- 定义与 Job 同事务冻结的初次启动记录、唯一关联与摘要；queued/uncertain/started 不等于 Job 执行状态。
- 外部发送前持久化 uncertain，旧租约 fencing、绑定实例原子性及取消不复活。当前上游不去重，不把 uncertain 自动恢复 queued。

## 2026-10-05 · 失败重试保留无关分支

- 单节点 Retry 不得重复已完成无关分支；引擎 seq 不代替冻结依赖图。无法隔离的直接重跑在所有副作用之前拒绝，不增加业务重试计数。

## 2026-10-05 · 重新生成与技术重试分离

- 明确反馈字节上限、冻结原文、幂等比较与旧定义能力门控；反馈只调整下一轮本节点提示词。

- 明确审批回执与步骤成功分离、显式幂等键的跨轮重放及兼容无键调用的边界；审批投影与接受决定使用同一聚合锁序，避免刷新竞争重新暴露已提交决定。

- 用户重新生成仅推进当前节点的审批循环，当前候选与状态取最新生成轮；旧轮保留审计，不增加失败重试计数。
- 精确等待任务决定新轮，不能将旧轮确认或旁路任务作为当前执行状态。
- 细化持久审批的实际 Task 身份、不可变候选/命令快照、幂等约束与领取 fencing；一个 Review 同时承担决定事实和待提交命令，不新增平行审批模型。

## 2026-10-04 · 原生执行输入引用生命周期

- 新提交的受控媒体输入冻结 Object Path，避免审批、排队和重试保留过期签名。
- Dispatch 快照保留稳定输入，实际交付和重取时重新解析；不让客户端执行模板渲染。

## 2026-10-04 · 普通提交的 Workspace 与企业校验

- 普通 User 新建 Job 也验证 Workspace 当前执行权限并冻结企业；绑定企业的 User API Token 不能覆盖为其他企业。
- Personal Poll 匹配 Job 企业，未归属或不匹配时不创建 Dispatch；不改写已有无归属任务。

## 2026-10-04 · Agent Workspace 执行隔离

- Agent Job创建持久化企业/Workspace；读写限定可信owner、企业、Workspace并核验当前执行权限，查询先鉴权后同步Conductor。
- 原生Job仓储不再丢弃已有企业/Workspace字段；列表查询追加受信隔离条件，不改写归属为空的历史记录。

## 2026-10-03 · Agent 委托执行归属

- Agent提交Job保留经Control核验的企业/Workspace并检查当前执行授权；Job读取、列表和操作追加企业限制，旧无企业Job拒绝。
- 不把User归属的普通Workflow定义描述为已实现Workspace资源模型；该模型差距单独跟踪。

## 2026-10-02 · 历史已完成视频终态

- 增加 `ImportCompletedExecution`：四个真实已登记输出同时成为成功 StepRun，Job 直接为 `succeeded`，没有 Conductor 实例；原视频时间与导入时间分离。
- 同一源 Video 的不同事实请求冲突，不允许覆盖终态；VideoPush 不塞进 Job 的单输出。

## 2026-10-03 · VideoPush 移出本轮迁移范围

- 已完成 Video 的终态 Job 不携带 VideoPush 成员/剪辑/发布事实，后者只留在旧源清单等待后续模型设计。

## 2026-10-03 · 独立历史发布聚合关联执行

- 用户随后确认 Historical VideoPush 单独迁入目标，并关联已导入的成员 Video Job；Execution 仍不保存多视频组合或发布状态。

## 2026-10-02 · Resume 输入引用的生命周期

- 历史 Job 与 Outbox 仅持久化受控 Object Path，不保存可能在队列等待时过期的签名 URL；交付时解析失败不产生 Worker Dispatch。

## 2026-10-01 · 历史未完成图片谱系可继续执行

- 完整上游但无下游 Video 的 ModelImage、StyleImage、FirstFrame 不再仅按普通 Generated Asset 处理；导入为 `awaiting_resume` Job，已有图片对应成功且标记来源的 StepRun。
- 明确历史时间/归属/来源 ID 与实际导入事实分离；断链源记录排除，重复导入按来源键幂等。
- 用户显式补齐输入后才能继续；历史成功 StepRun 不重做，新的 Conductor 实例只派发未完成阶段。导入接口与 Workspace 归属校验已实现；显式 Resume、持久化 Outbox 和完整联调仍待完成。

## 2026-09-28 · Personal 与 Managed Worker 路由

- WorkerDispatch 增加 `execution_target`、`actor_id`、`auth_source`；Managed Worker 的 `credential_id` 为空。
- Personal Worker 使用 API Token 委托身份和 User Domain；Managed Worker 使用 `internal_runtime` 身份和受控 Managed Domain。
- Managed Worker 不读取 Job User 后模拟登录，不请求 Talos；Job User 是业务归属，Worker Actor 是实际执行者。
- 两类 Worker 复用 Dispatch/Result Inbox 幂等链路，但认证入口和 Conductor Domain 严格分离。

## 2026-09-27 · Worker Gateway、Dispatch 与 Task Domain

- 客户端 Media Worker 不再直连 Conductor，改经统一网关与 media_workflow Worker Gateway API 领取、续租和完成任务。
- 新增独立 WorkerDispatch 聚合与 Worker Result Inbox；结果先在本地幂等接收，再异步更新 Conductor。
- Capability Task 在启动时通过 `taskToDomain` 按提交 User 隔离；禁止客户端指定 Domain、无域 fallback 或共享队列领取后再鉴权。
- 客户端只看到 `dispatch_id + attempt`；Conductor task/workflow ID 留在服务端。
- 输入 Object Path 在领取时转换为短时地址，输出先上传受控对象存储，再由 Finalizer 登记唯一 Generated Asset。

## 2026-09-27 · Product 图片进入执行输入

- JobInput 的 ProductData 新增 `image_url`，提交时冻结主图（无主图则首图）的可访问 URL。
- 编译执行时 Product 的 `asset_ref` 使用该 URL，Product ID 继续保留在快照中；图片顺序仍由 Invocation Mapping 决定。

## 2026-09-26 · Conductor HUMAN 审批闭环

- 编译结果冻结 `candidate_count/approval_required`，需要审批的 Step 在 Capability 与 Finalize 之间插入 HUMAN task。
- media_workflow 从 HUMAN input 投影候选与 `needs_confirm`；确认完成 HUMAN task，重新生成从 Capability task 原位 rerun。
- Finalize 完成后由 media_workflow 幂等登记 Generated Asset；客户端 media_worker 不执行审批、Renderer 或 Finalize。

## 2026-07-03 · 初始建模（media_agent v1）
- 由 `media_ai` 的 `job` / `videoTask` 等迁移并合并：
  - 旧版 job 字段 `"job input"` / `"job output"`（JSON）拆成 `jobs.inputs` 入参快照 + `step_runs.resolved_inputs / output_asset_id` 单步快照；不再冗余"整个 workflow 一份 JSON"，理由是 step 粒度快照便于排查与重跑。
  - 状态机由 `pending/running/completed/failed` 扩展为 `succeeded/failed/cancelled`，并独立维护 `started_at` / `finished_at`。
  - StepRun 与 Step 一对一快照模板版本号 → 模板改版后旧 Job 的 Render 仍可重现。
- 「业务规则变了比较多」中提到的"两边都要冗余 job 输入/输出"——本设计通过**只在 execution 域持久化**消除了冗余；workflow 域不存任何 job 维度的快照。

## 修改记录

### 2026-07-21 · 执行协议对齐外部 pull 模型 + 补单步能力
- 行为约束第 2 条由"内部按 step_order 逐个触发"改为"外部 worker pull（ClaimStepRun）+ advanceDAG 推进"，对齐 [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md) 与当前实现。
- 确认 `RetryStepRun` / `GetStepRun` 为正式能力（原已列能力清单但 proto 未暴露，本次补全到 proto + workflow_execution，见 [workflow_execution change.md](../../workflow/0002-workflow_execution/change.md)）。
- `FailStepRun` 语义明确：让在跑的前驱 step 跑完，仅后续 cascade skipped（见 workflow_execution §FailStepRun）。
- 跨服务可达性 RPC 统一经 `media_materials.reachability`（[ADR 0006](../../../adr/0006-reachability-layering.md)）。

### 2026-07-21 · bundle 模式对齐
- 行为约束 1 改为 SubmitJob 校验 bundle ↔ input_schema，**不重新校验可达性**（caller 在 SubmitJob 前已调 media_materials.reachability 校验）。
- Retry 权限：可达性不重新校验（bundle 自包含；retry 用原 bundle 或 input_override）。
- 确认 media_workflow 不调 reachability，仅 `BatchGetAssetKind`（asset_kind）+ `UpsertGeneratedAsset`（generated_asset）。
- 与上一条的关系：上一条假设 execution 调 reachability，本次按 bundle 决策校正。

### 2026-07-03 · 第三轮反馈：Job in-place retry 允许
- 加 `retry_count` 与 `retry_history` JSONB 列；新增 `POST /api/v1/jobs/{id}/retry` 与 `POST /api/v1/jobs/{id}/steps/{step_run_id}/retry`。
- 状态机由 "failed 终态" 扩为 `failed → running`（retry 触发）；其余终态不变。
- 上限 `MaxRetries` 默认 3，可在 service 配置。
- 用户反馈明确要求"in-place retry 可以"——旧版"新建 Job"语义保留，但本版补 in-place 通道。
- "旧 Job 重跑"语义改：默认走 retry 路径（旧 attempt 产物可被覆盖）；如要"全新跑"显式用 Submit 新 Job。

## 来自业务规则的强约束
1. `scope` 沿用 workflow.scope，禁止外部修改；提交时强制 workflow.scope == 任务上级 scope（来自调用方传入）。
2. cancel 是**幂等且终态**：running → cancelled 后台任务收尾后允许再写一次 `finished_at`，但不改变 status。
3. Output asset 落 `media_materials.Asset`，仅持 asset_id；产物二进制不在 execution 库，避免大对象影响 DB。
4. retry 时同一 StepRun 行的 `output_asset_id` 会被覆盖；旧 attempt 产物存放在 `retry_history` 与 audit log（不在本 spec 范围）。
5. **单一产出**：StepRun 持 `output_asset_id`（单值），与 Template 单一 `output_resource_type` 对齐；不再支持多产出列表。
# 2026-08-06

- 外部 Worker Pull 改为下游任务命令 + 完成事件；StepRun 只保存下游 service/task ID。
- Job/StepRun 增加 Workspace 边界，删除跨服务数据库外键。

## 2026-09-17 · 单输出下的多候选人工确认

- StepRun 继续只有一个 `output_resource_id`；下游多张图片建模为 Candidate，不扩展为多输出 Step。
- 新增 Step Review Decision，明确 `accept(candidate)` 与 `regenerate(feedback)` 互斥。
- 增加 confirming/regenerating 过渡态，使本地决定、下游 Generation 状态和最终资源登记可以通过 Outbox/Inbox 恢复。
- 重新生成递增 attempt 并保留旧候选；与 failed 后的 Retry 保持不同语义。
- 审批门槛由 Step `approval_policy` 决定：多 Candidate 始终审批；单 Candidate 仅在 `always` 时审批。
## 2026-09-17：Conductor 承担技术编排

- Job/StepRun 继续作为业务事实源，Conductor 只负责持久调度、等待和技术重试。
- SubmitJob 通过 Outbox 以 job_id correlationId 启动编译后的 Definition。
- Capability Worker 必须先幂等提交业务结果，再完成 Conductor Task。

## 2026-09-26：Job 输入按 Asset Kind 选择

- SubmitJob 的 material / asset bundle 必须与冻结 Input Schema 的 `asset_kind` 对齐。
- Job 创建界面消费服务端 Input Schema 生成资源选择器，不再要求业务用户手写 bundle JSON。
# 2026-10-02 · 纯文本动作作为历史继续执行输入

- Job Input 可冻结 `asset_kind=movement`、非空内容和空 URL；该模式不生成 Object Path，也不通过对象存储解析。
- 只有冻结的 Template Invocation 不引用 movement 的 `asset_ref` 时才接受纯文本动作，避免运行时把空媒体引用交给 Capability。
## 2026-10-05 · 原生启动取消协调

取消在本地先提交 Job 事实；未发送启动原子撤销，已发送但未知保留查询，已绑定实例进入 cancel_pending。后台确认引擎终态后保存取消审计，禁止通过启动回写复活 Job。外部终止与数据库提交分离，失联不等于清理完成。
## 2026-10-06 · 审批与失败重试周期隔离

- 将生成轮次与Execution Retry Epoch区分；新Review冻结接受时Job retry_count并在聚合锁内核对，旧Review不回填。
- 同周期阻止晚到投影，旧周期决定不影响重试后同数字generation重建的任务；显式命令重放保持原审计。
## 2026-10-06：初次启动精确对账

- correlation只定位候选，恢复及取消清理核对完整冻结命令，防止错绑或终止同correlation异源实例；不提供启动去重承诺。
# 2026-10-07 · 独立持久重试命令

- 新增Execution Retry冻结User请求、失败周期、原实例/编译、目标Renderer与重置Step集合；独立于初次启动和已经发生的重试历史。
- 同Job/key同请求返回原回执，同周期只能接受一条命令；接受不改Job计数，后台应用与applied必须同事务。原键重放仍受当前归属授权约束。
- 此结构是持久Retry的基础，不能用表/Repo完成代替引擎标识原子性、进程恢复与公开接口门禁。

## 2026-10-07 · 审批原生权威收敛

- 候选与资格来自当前原生HUMAN，审批事务只存决定，不提前claim Step或推进Job。
- accepted_retry_count保留审计与摘要，不作为准入条件；精确Task、retried标记、取消与归属继续隔离晚到决定。
- HUMAN完成后投影选择；送达HTTP成功仍查询精确Task输出，避免忽略更新被误记applied。

## 2026-10-07 · 取消与暂停状态投影

- native取消由原实例活动状态准入，旧失败/成功缓存不再否决；持久意图保存失败或缺少初次启动记录时不走旧直连终止路径。
- 增加非终态paused投影与兼容追加IDL枚举，原生TERMINATED投影cancelled；迁移24只扩展状态约束，不改变历史记录，存在paused时降级原子拒绝。

## 2026-10-07 · 输入覆盖重跑与来源快照

- 区分原生整次rerun与普通retry，以当前精确实例终态授权，不依赖缓存failed；确认新Input后只提交本地审计/查询缓存失效。
- Finalize优先使用本轮Conductor Input中的商品/人物来源，防止本地审计尚未提交而关联到旧对象。历史Assets/Review不删除。

## 2026-10-07 · 交付记账不拥有任务超时

- 移除本地Dispatch过期扫描与截止准入；lease_expires_at只作交付缓存刷新参考，任务有效性与超时由Conductor决定。
- 活动原Task允许绑定身份续心跳/交付；同Task/attempt重投保留原Dispatch ID，旧expired记录可兼容刷新，不改变执行者绑定或原生队列。

## 2026-10-07 · Worker产物对象上传契约

- 定义当前Dispatch尝试与候选位置绑定的写入准备，企业/Scope来自Job、输出媒体来自冻结Capability；候选对象仍须Complete/审批/Finalize才能成为业务产物。
- 不增加调度器或上传表，物理对象按任务和内容清单隔离；公开API/客户端实现与真实完整流程在独立plan跟踪，设计目标不等于已交付。
# 2026-10-07 只读执行关联元数据

启动JSON保存技术traceparent并参与原启动清单摘要；不进入用户输入/提交语义摘要，不作为幂等或归属键，不新增业务聚合。异步恢复详见Workflow；通用W3C技术概念不加入业务术语词典。
