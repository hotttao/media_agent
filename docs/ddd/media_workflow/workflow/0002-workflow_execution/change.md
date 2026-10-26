# workflow_execution 变更记录

## 2026-10-26 · Finalize 传递可信 Job 企业

首帧视频真实验收暴露通用能力缺少 Product/VIP 时无法登记；从已验证 Job 传递企业，只恢复失败登记，不重跑成功调用。

## 2026-10-07 · 结果交付责任移到客户端

同步代理原Task更新并查询确认，响应丢失不盲重发，不接受ignored terminal update为成功。Personal文件日志支持退出恢复；Managed同次执行内重试发送，进程恢复仍用原生重试和下游幂等。旧Inbox存档，不提供第二套执行状态。

## 2026-10-07 · Provider技术失败交给原生retry

Personal/Managed失败交付明确映射FAILED或FAILED_WITH_TERMINAL_ERROR，纠正统一terminal导致技术重试被屏蔽的问题。客户端只重发同一回执，不重跑Provider；引擎按冻结Task Definition创建技术尝试。新增真实Conductor分类门禁，其他租约/晚到交付边界仍未完成，不引入本地重试器。

## 2026-10-07 · Finalize登记先于原生Task完成

登记在Conductor调度的Finalize Worker中执行，不再依赖Get/List观察。Materials同键登记重放与Conductor技术重交付相结合，返回真实资产身份/稳定对象引用；Worker不更新业务执行状态或推进DAG。内部错误区分永久契约错误与临时服务故障，后者使用原生重试策略。

## 2026-10-07 · 原生Retry入口接线与缓存窗口撤出

Job/Step失败继续统一POST原实例retry，已RUNNING请求只查询，响应不明确不盲重发。Worker按原实例及Task状态直接交付，不等待本地Retry提交、不使用callback=2修补缓存。保留成功结果与已通过Human决定；旧rerun周期仅为输入覆盖兼容，不代表原生恢复周期。部署全流程、Finalize登记和旧调度器退役仍待完成。

## 2026-10-07 · 原生失败继续替代自建重跑恢复

普通失败恢复优先原生retry，人工重新生成使用HUMAN/DO_WHILE；不再接入发送租约、冻结重置计划或证据控制层。实际编译图的并行成功旁支/失败调用继续门禁通过；原误判为失败的空poll改为同实例异步JOIN观察，不新增补偿。公开接线和input_override兼容迁移仍需验收。

## 2026-10-07 · 重试只读恢复协调

- 冻结可比较基线，发送前核对原快照；uncertain恢复只GET，同一流程POST最多一次。
- witness要求新输入/原定义路由及完整Task重置，不依赖正常HTTP响应或RUNNING窗口。半写入证据等待，engine_confirmed仅交后续应用。

## 2026-10-07 · 重试投递领取和发送边界

- 领取与短事务标发送分离；外部Rerun只能发生在uncertain提交之后，恢复领取不产生第二次POST。
- 所有租约写入按token和数据库有效期fencing，释放退避不改业务事实；取消后的晚到引擎清理继续独立验证。

## 2026-10-05 · 独立生成结果的跨服务登记

Finalize 使用稳定结果键登记独立产物，同轮重放原 ID、新轮新 ID。Materials Asset/回执同事务，外部登记与 Execution 当前引用保存分属不同事务；登记成功而本地失败时重放原键恢复，不覆盖历史 Asset。用户已采纳该语义，替代默认覆盖方案。

## 2026-10-05 · 原生 Submit 持久启动

- 启动记录与 Job/StepRun 同事务，网络发送在事务外；发送之前 CAS 为 uncertain，只有 correlation 查询可恢复不明确结果。
- 取消协调、上游幂等及服务装配分别验收，不以仓储门禁替代公开 Submit/后台执行。

## 2026-10-05 · 审批驱动的局部重新生成

- Renderer 通过循环参数获取上一轮审批输出并校验相邻轮次，仅追加当前反馈；新定义声明 feedback mode，旧定义非空反馈明确拒绝。

- 用户接口先落库返回 202 回执，后台独立投递；显式键重放不查询新轮 HUMAN。投影与决定事务串行化，不以先读后写代替并发保护。

- Renderer/Capability/Approval 编译为每个 Step 独立循环，Finalize 只在接受本轮结果后执行；不调用非终态 rerun，也不终止并行 Workflow。
- 精确识别最新循环轮次与等待 HUMAN，保留旧轮审计但不允许旧候选覆盖当前投影；旧不可变定义不原位升级。
- 真实 Conductor 协议测试证明新 Task ID、新候选及最终引用，并确认并行 HUMAN 保持原身份与状态；业务接口验收独立执行。
- 明确 Review 与退出待确认在同一事务保存；后台先查询精确任务、再提交或核对，领取重放使用 claim token 防止旧进程覆盖。

## 2026-10-04 · 新任务媒体交付重签

- 将稳定输入引用与运行期交付地址分离，普通 Submit 不再冻结媒体签名；纯文本内容保持不变。
- 首次解析失败不创建 Dispatch，已有租约在重取解析失败后仍可恢复，且不得重 poll 新任务。

## 2026-10-04 · 功能先行与新任务归属

- 暂缓历史核对和迁移、UI 实现以及 Agent 扩展，先收口普通提交与 Worker 执行功能。
- 普通提交增加目标 Workspace；由 Control 校验授权和真实企业，Worker 交付再次匹配 Job 企业。CLI 和单样本门禁同步请求契约。

## 2026-10-02 · 已完成视频独立导入

- 每条来源 Video 对应一个终态历史 Execution；共享上游图片可被多个 Job 引用但 Asset 只登记一次，当时曾规划 VideoPush 多视频关系另行保存。
- 视频及三张上游图片先按来源 ID/摘要摄取，再由单个事务保存四个历史成功 StepRun 和 `succeeded` Job；导入不启动 Conductor，失败可按来源键重试。

## 2026-10-03 · 暂缓 VideoPush 迁移

- 用户决定本轮不迁移 VideoPush；多视频剪辑与发布流程待单独设计。已完成 Video 仍按源 ID 独立导入，VideoPush 只保留在旧库只读清单中，不建立目标读模型。

## 2026-10-03 · VideoPush 改为独立历史记录

- 用户随后明确采用独立 Historical VideoPush 记录：成员关联目标已完成 Video Job，原文字、状态、时间及媒体迁入目标；未来剪辑/发布流程仍待单独设计。
- 当前四节点 Workflow 及其单输出不承担 VideoPush 导入或发布副作用。

## 2026-10-02 · 恢复输入使用稳定对象引用

- 用户提供的短期签名 URL 仅用于 Resume 时校验企业命名空间和对象可读性；Job/Outbox/Conductor 输入冻结 Object Path。
- Worker 领取历史 Capability Task 时由 media_workflow 经 media_materials 内部只读解析接口重新签发 URL；解析失败不交付，不复用过期 URL。

## 2026-10-01 · 导入未完成执行与显式继续

- media_ai 图片链只要上游引用完整，即使没有视频，也需按最深完成阶段恢复为未完成 Workflow Execution；用户后续可测试继续执行。
- 明确对象摄取、导入 Job/StepRun、用户 Resume、Outbox 启动新 Conductor 实例、已完成节点受控 restore 的跨事务时序和幂等键。
- 不用 `SubmitJob` 伪造历史、不自动重跑历史图片；真正断链记录不导入，原系统保持只读。

## 2026-09-28 · Personal/Managed Capability 执行

- Capability Version 冻结 Execution Target；Job 启动分别分配 Personal User Domain 或 Managed Pool Domain。
- Personal Worker 使用绑定 Worker Actor 的 API Token，JWT 为 `sub=User + act.sub=Worker`；Managed Worker 位于 media_workflow 内部，不使用 Talos或 API Token。
- 两种 Worker 复用 Dispatch、lease 和 Result Inbox，但不能互相领取或自动回退任务。
- Managed Worker 记录 Job User 与执行 Worker Actor 两个维度，不读取 Job User 后冒充登录。

## 2026-09-27 · 统一网关与 Conductor 代理协议

- 客户端 `media_worker` 从直连 Conductor 改为携带 API Token 经统一网关访问 `/api/worker/v1/*`。
- media_workflow 按 Capability Registry 和 User Execution Domain 代理 Conductor poll，领取后先持久化 WorkerDispatch 再交付。
- heartbeat 对齐本地 lease 与 Conductor response timeout；断线不直接终止 Job，过期任务可安全重投。
- completion/failure 先写 Worker Result Inbox 并返回稳定 receipt，再由后台 Adapter 重试更新 Conductor。
- 浏览器执行接口继续使用 Web Session；Personal Worker API 使用 API Token 换取的委托 Internal Identity JWT，不依赖 Kratos Cookie。

## 2026-09-27 · Product 图片输入映射

- Product 主图（无主图则首图）的可访问 URL 进入 JobInput 快照并映射到 `asset_ref`；Invocation Mapping 决定它在 Capability 图片数组中的顺序。
- gpt_image2 原始提示词正文不附加 URL，素材引用由固定的调用映射单独传递。

## 2026-09-26 · Executable IR 审批实现

- `ExecutableStepIR` schema v2 增加候选数量与是否审批的冻结结论。
- Conductor 以 HUMAN task 表达可长时间等待的用户决定；接受候选完成 HUMAN，重新生成 rerun 对应 Capability task。

## 2026-07-21 · 改为外部 worker pull 模型 + 补全执行协议

- 触发点从"内部 worker 拉起"改为**外部 worker pull**：下游平台 worker 主动调 `ClaimStepRun` 拉取 step_run，调 `CompleteStepRun` / `FailStepRun` 回写。解耦调度与执行，避免服务内嵌执行器（对齐当前实现）。
- 补全执行协议各阶段：SubmitJob、ClaimStepRun（`FOR UPDATE SKIP LOCKED` 原子领取）、CompleteStepRun、finalizeStepRun、FailStepRun、ConfirmStepRun、RegenerateStepRun、RetryStepRun、GetStepRun、超时回收 scanner、advanceDAG、checkJobCompletion。
- 补 `RetryStepRun` / `GetStepRun`（原 [execution](../../domain/0003-execution/execution.md) 能力清单已声明但 proto / 本 workflow 未暴露，本次补全到设计与实现）。
- 厘清 `RegenerateStepRun` vs `RetryStepRun` 边界：前者 need_confirm -> pending 重新生成候选，后者 failed/skipped -> pending 重跑单步 + 下游。
- 厘清 `RetryStepRun` vs `RetryJob` 边界：前者单步 + 下游，后者整 Job。
- `FailStepRun` 语义修正：让在跑的前驱 step 跑完（不中断已 claimed 前驱），仅后续 cascade skipped（原实现立即 Job failed，本次对齐文档语义）。
- 跨服务 RPC 统一指向 `media_materials.reachability`（AssetReachability / Check* / ResolveAssetByKind / AssetRead / UpsertGeneratedAsset），见 [ADR 0006](../../../adr/0006-reachability-layering.md)。

## 2026-07-21 · 调用方对齐 bundle 模式

- 渲染改为用 caller 组装的 bundle（含 url/content），**不调** `ResolveAssetByKind` / `AssetRead`。
- 跨服务 RPC 列表只剩 `BatchGetAssetKind`（校验期）+ `UpsertGeneratedAsset`（产出期）；删除 AssetReachability / Check* / ResolveAssetByKind / AssetRead（这些归 caller 调 reachability workflow，见 [media_materials reachability](../../../media_materials/workflow/0001-reachability/reachability.md)）。
- §3 解析改为从 bundle 取 slot 值（caller 组装 bundle 时已调 reachability 解析 product/vip 输入 + 拿 url/content）。
- `RetryStepRun` 的 `input_override` 覆盖 bundle 部分值；可达性由 caller 在 retry 时重新校验或信任原 bundle。
- 与上一条的关系：上一条假设 execution 调 reachability，本次按现有 spec/实现的 bundle 决策校正（media_workflow 不调 reachability）。
# 2026-08-06

- Workflow 执行改为 Outbox 下发幂等任务命令并消费结果事件，不再暴露 Claim/Complete/Fail Worker Pull 协议。

## 2026-09-17 · 多候选确认与重新生成

- 多候选完成事件不再直接推进 DAG，而是让 StepRun 进入 needs_confirm。
- 用户可以接受唯一 Candidate，或携带 feedback 请求重新生成；两种决定按 StepRun attempt 互斥且幂等。
- 确认与重新生成均采用“本地决定 + Outbox 命令 + 下游结果 Inbox”三段式，不在本地事务内调用 Generation。
- accept 只有在候选确认和 Generated Asset 登记完成后才推进后继；regenerate 创建新 attempt，旧候选与晚到事件只保留审计。
- Step 可配置 `approval_policy=always` 让单 Candidate 也进入 needs_confirm；多 Candidate 不允许跳过审批。

## 2026-09-17 · 改由 Conductor 持久编排

- Workflow 发布时编译为 Conductor Definition；Job/StepRun 继续是业务事实源。
- 一个业务 Step 编译为 `media_step_execution_v1` 子工作流，内部以 DO_WHILE 承载带反馈重新生成。
- 多节点拓扑波次使用动态 fork + JOIN；后继从 StepRun 读取前驱唯一输出，不依赖 fork 输出结构。
- 外部 Worker 不再直接领取 StepRun，而是经受控 Worker Gateway 执行 Capability Task。
# 2026-10-02 · 纯文本动作输入分支

- Resume 的 movement 输入分为媒体型和纯文本型。前者验证企业对象并冻结 Object Path；后者仅冻结已授权素材内容，不调用对象存储，也不生成虚构的媒体引用。
- 运行时仍依据已发布 Template Invocation Mapping 组装 Capability 输入；冻结版本若要求 movement 资源引用，拒绝纯文本输入而非隐式改写旧版本。

## 2026-10-05 · 失败重试分支隔离

- 真实 Conductor 证明指定任务 rerun 会截断按 seq 位于目标之后的已完成旁支，不能当作 DAG 局部重试。
- 直接路径增加副作用前安全检查；完整原生并行恢复需可靠重试意图、冻结依赖及受控结果恢复，仍待实现，不套用历史导入身份。
## 2026-10-05 · 启动和取消的竞态

明确本地取消事务、后台只读恢复和引擎终止的顺序。queued 撤销、uncertain 查原实例、started 转 cancel_pending；终止响应丢失先读终态。找不到未知实例不能关闭清理义务，取消重放不重复启动。
## 2026-10-05 · Submit 同键重放

授权先于回执查询，规范化媒体引用先于摘要比较，原回执重放先于媒体读取和当前发布版选择。并发首次接受在事务内锁键/二次查询，聚合、启动记录与回执一起提交。此去重不替代上游不明确启动的只读恢复。
## 2026-10-06 · 审批重试周期贯穿投递与投影

- 接受事务冻结并核对Retry Epoch，后台不向新周期投递旧决定；节点/Job投影同时核对周期、生成轮及精确审批身份。
- 保留旧摘要及未知周期，不以猜测回填掩盖迁移歧义；本次不替代可靠Retry恢复流程。
## 2026-10-06：初次启动精确对账

- 唯一候选必须再读取实例、核对Definition版本/输入/Task Domain。缺字段、不同命令、重复实例不绑定/终止，不自动重发。
# 2026-10-06 · Provider契约交付校验

- 领取及重交付执行发布schema、Capability引用、参数类型/资源数量校验；新媒体参数限制企业受控对象，解析HeadObject ContentType/SizeBytes检查媒体及非空，同时刷新签名URL。
- 客户端新增独立Provider fixture profile，以精确Job/Step配置已知结果，不按业务节点命名定义底层能力；内部Render仍由media_workflow执行。旧profile保留，不能混用覆盖参数。
- 校验失败不创建新Dispatch；Conductor已领取任务依其超时/重试处理，不把参数拒绝当作生成结果成功。
# 2026-10-07 · 重试窗口内延迟交付

- 对已归属native Job仍failed、但原引擎实例已running的任务，核对唯一精确活动任务和完整输入后用原Task ID回调延迟2秒；不创建Dispatch、不返回媒体、不中断旁支。
- Personal/Managed都保留身份/实例/输入约束，取消、成功、错实例和歧义不重排队。只补偿交付窗口，不代表持久Retry意图和崩溃恢复已经完成。
# 2026-10-07 · 持久执行重试协调

- 在Rerun前持久接受命令，发送前标uncertain；通过原实例的冻结标识及执行变化证据恢复，不依赖RUNNING或盲目重发。
- 冻结计划来自Job原Compilation，应用和命令applied同事务；终态基线与引擎证据仅摘要化保存，不冻结签名URL。标识原子性及取消/晚到操作必须用真实引擎门禁验证。

## 2026-10-07 · 审批送达与原生状态分离

- 当前HUMAN输入是候选源；本地缓存不再禁止原生活动任务。接受事务只写决定，执行状态由引擎完成后投影。
- 精确Task ID与retried标记替代本地周期准入判断；取消意图、身份与实例关联仍校验。
- 提交成功必须GET核对精确Task COMPLETED且内容相同；响应丢失保留决定并查询恢复，不替换目标任务。

## 2026-10-07 · 取消原生实例准入

- 已绑定native实例取消前查询RUNNING/PAUSED，已取消重放不依赖引擎；本地只提交取消意图，由现有后台确认原实例终态。
- 取消清理额外核对GET返回实例身份，防止异常读取终止/确认其他实例。
- 暂停独立投影为paused，终止为cancelled，不用候选审批或技术失败状态表达。

## 2026-10-07 · 原生整次rerun确认

- Job input_override保留明确的原生整次rerun，GET前置终态校验与提交后输入/变化确认；丢响应不以RUNNING猜测，也不自动重发。
- 本地事务只使查询缓存失效并保存输入/审计；Conductor拥有旧Task清理与重跑调度，不保留局部重置计划。跨系统提交不承诺原子性。

## 2026-10-07 · 原生Worker超时边界

- Personal/Managed结果与心跳不再由本地90秒字段否决；保留精确Task、尝试、归属和取消校验，心跳代理后再次确认原Task活动。
- API与Managed启动移除本地Reaper，DAL不再按本地时钟批量过期；同身份原生重投只刷新记账，不创建Task或尝试。终态结果仍查询确认，晚到替代结果不重定向。
# 2026-10-07 异步追踪关联

同步HTTP已有W3C关联，初次启动后台和客户端/内部Worker仍断链。冻结并恢复纯traceparent技术元数据，以现有本地非采样span串联日志；不启动可观测性、不改发布图、不新增状态/队列或权限，不以trace决定执行。重放与rerun保留原关联，缺失不回填历史记录。
# 2026-10-10 · 新执行复用的编译分支

补充内部匹配、Materials 可读性校验及 Conductor SWITCH 跳过生成/审批的目标时序；复用证据不替代原生调度，Finalize 幂等登记后才产生可信索引。代码接线与真实协议验收尚未完成。
