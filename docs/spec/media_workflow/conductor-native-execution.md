# Conductor 原生执行边界

## 范围与权威

Conductor拥有Task/Workflow的执行状态、调度、技术重试、并行、JOIN、循环、人工等待与失败恢复。业务服务只做DSL/Executable IR编译、模板渲染、Capability Worker代理、资源登记、身份授权、审批决定和执行查询投影。不新增第二套执行命令/租约/任务重置状态机。

## 操作语义

| 操作 | 原生实现 | 业务服务职责 |
|:---|:---|:---|
| 调用失败自动重试 | Task Definition retry策略 | 配置能力超时/重试，Worker依原Task/attempt回写 |
| 失败后继续 | `POST /api/workflow/{id}/retry` | 授权原实例、查询状态、转发命令、刷新投影；保留成功任务和现有输入 |
| 审批要求重新生成 | HUMAN决定 + DO_WHILE | 决定/反馈提交精确Human Task，结果由循环推进 |
| 改变输入重新开始 | 原生rerun/restart或显式新执行 | 与失败继续区分；旧接口input_override不得静默删除或伪装为普通retry，兼容迁移单独验证 |
| 停止执行 | 原生terminate | 授权、转发、记录取消意图和查询结果 |

Job/StepRun保留归属、输入/版本关联、当前产物、审批/Worker结果的业务幂等和查询投影。投影落库事务不包含Conductor调用。缓存failed不是执行不可恢复终态；缓存running也不授权构造任务。普通失败恢复不清空候选/当前产物、不重置业务DAG、不调用renderer的rerun；原生Task ID/技术retry_count与Human Task/循环轮次区分旧结果。

## 实现与迁移

RetryStepRun以StepRun定位所属Job，然后执行与RetryJob一致的原生失败继续；不要求缓存Step为failed/skipped，不保证只恢复选中节点。仅FAILED/TIMED_OUT发出retry；RUNNING只查询，不把PAUSED/TERMINATED/COMPLETED自动转换为其他操作。Step input_override仍明确拒绝并引导整次rerun；Job input_override暂保留旧显式rerun兼容路径，不称为普通retry。

原生retry审计不得修改执行状态、清空节点或增加旧整次rerun周期。查询忽略Conductor已标为retried的旧Task，技术retry_count来自当前Task。Worker首次交付、重取、活动心跳/结果接受核对原实例RUNNING、精确活动Task、原生retry_count+1；缓存failed/succeeded不能否决。删除缓存窗口callback延迟；取消意图仍单独保护，完整取消/投影权威收敛另验收。

新增窄FailureRetryEngine端口和Conductor RetryWorkflow adapter；应用操作读取精确原实例，以引擎状态决定是否继续，不用本地周期驱动任务。对于已running的失败继续请求返回现有状态，不重复POST。响应不明确先查询原实例，不自动改命令、创建实例或宣称本地缓存更新代表引擎成功。

撤出未部署的execution_retries/migration024、RetryBaseline/Witness和retry_delivery；不删除已部署数据，不修改Conductor外部源码。现有普通Retry中的手动重置与投影终态限制、Worker failed缓存交付限制分阶段替换并验证。现有input_override和旧接口保留至对应兼容契约验证，不以删能力完成边界收敛。

## 验收

### 资源权限入口

Template/Workflow共享权限沿用既有owner/viewer/editor及部门Grant契约，不由Conductor代替。公开handler必须从认证上下文取得caller与部门事实，不能使用请求中的granted_by授权。GrantReq字段4保留但deprecated/ignored；Grant写入的审计身份是实际caller。模板更新命令带CallerID并在版本写事务内复核，Publish/Delete只允许owner；Update/Archive最终需支持editor+。GetTemplateVersion公开入口先校验资源可读性，内部冻结快照端口不得直接开放给客户端。

部门Grant消费、组织边界及授权列表分页需另验收，不能用owner守卫通过宣称editor/viewer共享完成。已实现的第一阶段所有者检查只是封住无鉴权写入，不改变DDD的最终editor+要求。拒绝请求不得写版本/状态/Grant或返回私有版本正文；handler伪造query/body中的granted_by需明确拒绝，并验证真实owner仍可正常操作。

### 编译前版本与必填输入校验

Workflow validator以`(template_id, template_version)`缓存和读取Slot快照，不能由同ID的其他版本替换。共享WORKFLOW_INPUT的required是全部消费Slot.required的逻辑OR：任一必需消费方存在就必填，仅所有消费方可选时才可选。resource_type和material/asset的asset_kind仍须唯一且一致；不改变拓扑序列、单sink或wave屏障契约。

回归须包含同一Template的两个不同版本使用不同Slot、错误版本Slot被拒绝、required/optional消费顺序互换与全可选入口。仅修正新验证/发布过程，不回填既有Workflow schema、Compilation或运行输入，不新增执行状态机。

### Provider失败分类

IDL `FailWorkerTaskReq`新增可选string `failure_kind=6`，值为空/permanent/transient，未知值返回400。handler只转Command；直接代理原生FAILED或FAILED_WITH_TERMINAL_ERROR，失败回写不携带output。成功响应为200/submitted，经原Task查询确认而非Inbox接受；完整客户端恢复契约见[直接交付spec](../worker-result-direct-delivery.md)。无新增表或schema迁移，历史Inbox保留但停止运行期写入与扫描。

Personal客户端支持FailWithKind及`Permanent() bool`错误接口（errors.As处理包装错误），明确非永久错误发送transient；未分类fixture错误保留永久失败。Managed Fail旧入口保留永久语义，新增FailWithKind；内部executor使用既有taskerror，永久错误停止原生自动retry，其他异常报告临时失败。无执行器内自动重试，无本地Job/Step重置。重试次数/延迟取冻结的Conductor Task Definition，不改变旧发布版本。

验收覆盖：非法分类无副作用、永久/临时映射、旧回执JSON兼容、独立客户端请求透传、Managed分类以及真实Conductor自动retry新Task身份/成功完成与永久失败不重试。技术门禁仅使用唯一测试定义和受控持久端口，不冒充公开HTTP/真实数据库/独立Worker完整验收；完整回归仍列N4。

### 内部Finalize与查询投影

`conductor.Runner`为Handler注入只读原生Task身份上下文，Finalize使用现有`job_id/step_key/candidates/selected_candidate/output_asset_kind`输入，不改冻结Definition。加载原Job/Step并核对企业、owner、版本/产物类型；GET原实例，核对精确活动Finalize、当前轮唯一成功invoke及候选。使用`GenerationResultKey(workflow_id, step_run_id, successful_invoke_task_id)`向Materials登记，成功后返回真实`output_resource_id`与`output_resource.id`，媒体引用为企业/Scope命名空间内稳定Object Path。不得以FinalizeTask ID或候选内容散列代替生成身份。

内部Worker使用Materials内部凭据，StorageOrigin使用既有`MEDIA_STORAGE_PUBLIC_ENDPOINT`。对象路径/URL规范化为受控、无签名的URL供现有登记API验证，并把稳定引用返回Conductor供后继领取时重签；纯文本不解析对象。对象类型/大小由Materials存储校验负责。跨企业/Scope、旧候选、错误Task/输出类型或缺少身份拒绝登记。

Get/List/GetStepRun仅读Conductor真实Finalize结果及已有业务引用，事务内投影ID/状态，不调用Materials Upsert。登记失败不能回写Task COMPLETED；登记已提交但回写失败由原生Task重试/重交付恢复同键ID，不新增执行控制层。已有终态Step引用保留；未登记旧占位输出明确拒绝，不能通过查询补登记。

内部错误使用稳定安全码：永久输入/身份/候选错误映射FAILED_WITH_TERMINAL_ERROR，临时外部不可用映射FAILED；Runner不自行执行任务重试。HTTP结果未知时同键Materials登记可以安全重放，不能换生成键。

测试必须证明：无用户查询也能完成Finalize和原生Workflow；登记已提交但响应丢失→原生自动retry返回同一Asset；Finalize新Task ID不改变结果键，新invoke新轮产生新Asset；失败/跨归属/旧轮候选不登记；GET无登记调用。隔离测试不得启动真实Provider或改既有用户执行。

旧ClaimStepRun/CompleteStepRun/FailStepRun地址保留兼容，IDL标deprecated，扩展中间件和Handler自身均返回410。Service不提供本地Claim/Complete/Fail或旁路Confirm/Regenerate，不保留ClaimAtomic/ReclaimStale仓库端口、本地DAG推进、超时扫描和无引擎Submit回退。缺少Conductor或注册Compilation首次Submit明确拒绝，不写本地任务；新Job/Step只能通过持久初次启动及原生执行路径处理。审批继续使用DecideReview，生成登记归内部Finalize，不从Job查询或旧完成API触发。

旧CLI本地claim/complete全链路测试退役，替换为不创建资源的网关410门禁；原生复杂流程由现有独立CLI/Personal Worker capability门禁验收。旧测试要求的本地DAG级联和直接落产物不作为当前功能保留，测试源码可由Git历史恢复，业务数据不删除。输入覆盖rerun、原生查询投影和历史导入/恢复保持独立，不用此清理掩盖未完成边界。

## 整次输入覆盖rerun兼容

Job input_override前置GET要求精确原实例FAILED/TIMED_OUT，不信任Job failed缓存。冻结输入只来自原注册Compilation/Template版本，受控存储校验不修改调用方对象。事务外POST原生整次rerun，再GET核对相同实例/Definition和完整新Input。成功响应但未观察结果报未确认；丢响应还需Input改变或旧Task ID被移除作为变化证据，不能只凭RUNNING/原来就相同的Input猜测成功，不自动重发。

确认后commitRerunProjection使用必需的本地事务，Job→Step锁序与取消/实例/编译/历史周期校验，原子写新Input、历史审计与旧Step查询缓存失效。不新建任务、不重置原生Task、不计算DAG影响集合；资产和Review历史保留。再调用Get投影实际原生状态。本地提交失败不声称引擎未执行，不承诺跨系统原子性，返回安全的未确认错误。

内部Finalize来源Product/VirtualIP以原生workflowInput.inputs[name].value.id为准；当前信封中未提供来源则不继承旧Job来源，格式错误在登记前拒绝。旧没有inputs信封的Definition才回退原JobInput。这样生成来源不依赖rerun本地审计提交的时机。

## Worker原生超时与交付记账

Heartbeat/Complete/Fail核对原精确Task与attempt、归属、取消意图；活动原Task不因本地lease_expires_at过期被拒绝。Heartbeat事务外代理IN_PROGRESS/callback并再次确认Task仍活动，之后只ExtendLease记账；结果仍按原Task状态/内容确认。旧expired记录可续记账，但结果终态记录不能通过poll变回执行中。

API与Managed不启动RunLeaseReaper，删除ExpireLeases运行端口与DAL扫描。Conductor拥有Task timeout、response timeout、技术重试与重投；本地不根据90秒时钟排队/失败Job。同身份原生重投相同Task+attempt时，Create保留原Dispatch ID并刷新交付时间，不分配新尝试或重绑定不同身份。FindActive的时间条件仅是响应缓存优化，原生资格仍单独复核；lease字段保持DTO/数据库兼容。

## 取消与原生状态查询

取消已绑定native Job时先读当前实例，RUNNING/PAUSED才接受新意图；已cancelled重放无需GET。旧失败/成功缓存不作拒绝条件。持久CancelInitialStart原子锁Job→启动记录，保存cancelled和queued撤销/uncertain查询/started清理义务；native没有记录不回退直接Terminate。后台核对精确实例身份，查询或终止未知时保留义务，确认原终态后完成清理。

查询PAUSED→JobPaused，TERMINATED→JobCancelled，FAILED/TIMED_OUT→JobFailed；暂停不是审批，不创建暂停操作API。IDL JobStatus追加PAUSED=8，不改变旧编号；migration 000024扩展jobs_status_check包含paused，降级存在paused时拒绝、保留原数据。代码部署前必须先完成向上迁移。

## 审批原生事实与业务决定

DecideReview先完成既有身份/归属校验和显式键回放；首次决定查询RUNNING实例的最新、唯一、IN_PROGRESS且未retried HUMAN，以其冻结candidates选择数组位置。Step状态/候选和Job失败缓存不是准入条件。无候选、错Task类型、非活动/歧义Task、取消意图拒绝；上游故障只返回安全错误。

AcceptReview事务锁Job→Step，仅校验关联、owner、实例与取消意图，原子保存Review；同Task第二决定或异摘要同键冲突。不得修改Step status/selected_index或Job status；accepted_retry_count仅保留不可变审计。已确认HUMAN的选择由原生查询投影，pending回执不等于执行已推进。

Review Delivery只向冻结Task ID提交；retried/替代目标拒绝。HTTP成功再次GET核对COMPLETED和相同输出才applied，未知保留pending；响应丢失先查询恢复，内容冲突failed，不重定向到新HUMAN。传输重试与Conductor任务重试不同，不调度生成。取消事务与接受事务串行，锁外发生取消由送达复核阻止。

实际编译器IR生成FORK_JOIN/DO_WHILE/SWITCH：完成旁支B、A调用失败、原生retry只继续A调用、保留A已渲染提示词与B完整Task身份/结果、JOIN异步推进、合流完成。空bounded poll不等于失败，不更换测试实例。仅别名化SIMPLE Task队列避免干扰实际Worker，不改图/表达式。随后需公开Gateway HTTP、独立CLI与真实Worker的一条完整业务流程；技术探针不替代部署验收。UI/全量历史/Agent不在本轮范围；启动幂等扩展仍暂缓。
