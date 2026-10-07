# Workflow 执行边界完整审查

> 本文下方为整改前审查证据，不是当前部署状态。后续核心整改、公开客户端流程、异步关联及 input_override/真实本地时间边界已完成，见[当前计划与逐项证据](../../../../plan/media_workflow/conductor-native-execution.md)和[设计说明](../../../../wiki/workflow-conductor.md)。部门/角色/资源权限扩展明确暂缓；引擎级启动幂等未实现。不删除历史发现，也不把曾经存在的本地调度代码误认为仍在运行。

日期：2026-10-07。事实基线：`eb75d53`上的当前未提交工作区，不把仅存在于工作区的adapter当作运行服务已经接线。审查标准来自用户最新约定：**业务核心是编译生成Conductor Definition；此后的Workflow执行语义由Conductor拥有，运行服务是授权代理和结果展示，不再建第二套执行引擎。**

本次只审查并形成报告；不改业务代码、IDL、迁移、部署或用户数据。UI、Agent扩展、全量历史迁移不进入整改范围。

## 结论

**理解的目标边界已经明确，现有实现尚未符合。** 编译主链已使用Conductor的FORK_JOIN/JOIN、DO_WHILE、SWITCH、HUMAN；运行侧却仍存在本地调度器、手动重跑/重置、缓存终态守卫和依赖查询的业务副作用。此前不断增加恢复代码，主要是在补偿这些双重权威，不是因为Conductor没有重试或循环。

不能只把Retry换个HTTP端点就宣布完成。必须同时收敛查询、Worker交付、审批和资源登记，否则原生retry已成功，本地仍会拦住任务或拒绝刷新状态。

## 审查范围与方法

- 枚举全部6份Workflow IDL：44个RPC（Workflow 10、Template 10、Job 15、Worker 5、Historical Publication 3、Health 1），追踪对应handler及路由/auth。
- 追踪`media_workflow/main.go`和Internal/Managed Worker启动入口，逐项审查publication/start/review/resume投递、结果提交、Dispatch lease reaper、旧Step claim scanner及Worker循环。
- 审查业务校验→Semantic Compiler→Executable IR→Conductor Backend→版本发布；区分现用`CompileExecutableIR`与旧`compiler.Compile`。
- 沿执行写路径核对`service/job`、Job/Step/Review/Dispatch/Start/Resume仓储和模型/迁移；逆向查找调用引擎、修改状态、推进DAG和外部资源登记的入口。
- 核对Personal/Managed Worker代理、`media_worker`客户端、内部Render/Finalize/Restore任务及Conductor client/runner；核对相关单测与已有真实引擎门禁的覆盖含义。
- 生成代码以IDL及实际路由接线核对，不将逐行审查自动生成DTO等同于执行边界审查。其他服务仅追踪实际涉及的网关授权/Materials/Control端口，不扩大为整个仓库所有应用的安全审计。

## 必须固定的权威边界

| 内容 | 唯一权威/所有者 | media_workflow允许承担的职责 |
|:---|:---|:---|
| DSL、Template/Capability版本、slot_bindings、可访问性 | 业务平台 | CRUD、契约校验、版本冻结、编译 |
| DAG依赖、分支、JOIN、循环、执行顺序 | Conductor Definition及运行引擎 | 编译表达；不能运行期自行计算可执行节点 |
| Workflow/Task运行状态、技术重试、超时和重跑 | Conductor | 授权后代理原生操作、读取状态；不按本地状态再调度 |
| HUMAN是否等待、循环当前轮次 | Conductor | 展示当前精确Human Task，代理用户决定/反馈 |
| 用户是否有权操作、企业/Workspace/Worker归属 | 平台身份与资源权限 | 最终授权和Task到业务归属映射；Conductor不是业务权限系统 |
| 图片/视频及Generated Asset身份 | 对象存储与Materials | 在Conductor驱动的业务Worker中登记并返回稳定ID |
| 审批决定审计、命令交付回执、结果去重 | 代理/业务边界 | 可保存最小记录；其状态只能表示“接受/投递”，不能驱动DAG |
| Job/StepRun展示状态 | Conductor的读模型 | 可缓存/投影，但缓存不能否决引擎事实或成为调度源 |

目标运行关系：

```text
业务DSL + Template/Capability → 编译 → Conductor Definition
                                         │
用户操作 → 网关授权 → media_workflow代理 → Conductor执行
                                         ├─ Render Worker：渲染
                                         ├─ Capability Worker：执行Provider能力
                                         ├─ HUMAN / DO_WHILE：审批与重新生成
                                         └─ Finalize Worker：登记资源、返回真实产物ID
用户查询 ← 权限过滤/展示映射 ← Conductor状态、Task输出及资源关联
```

代理层可以重试网络交付，但不能因此设计新的业务尝试、清空节点、恢复分支、决定完成或模拟引擎调度。取消请求已接受与引擎已TERMINATED也必须区分，不能把业务意图伪装成引擎事实。

## 必须整改的发现

### F01 · P0：普通失败恢复仍使用rerun，随后本地手动重置

证据：`media_workflow/biz/service/job/service.go:634`的Retry调用`RerunWorkflow`（668），711的RetryStepRun选择Renderer后调用`RerunFromTask`（776）。`retry_commit.go`清候选/产物/claim、重置Step并增加本地计数；`retry_impact.go`和`executable/retry.go`另算受影响集合；`retry_isolation.go`为按seq截断增加旁支拒绝条件。`dal/repo/job/job.go:163`的AppendRetryHistory同时把Job改running。

影响：无输入覆盖的普通失败继续也重跑已完成步骤/提示词；有成功旁支时本地隔离条件会拒绝恢复。此前设计的分支恢复和持久Retry计划是在修补错误的操作映射。

整改边界：普通失败继续代理原生`retry`；不手动重置DAG。输入改变/整次重新开始与失败继续分开。新`FailureRetryEngine`/`Client.RetryWorkflow`只有adapter和技术测试调用，**尚未被Job API使用**，不能宣称已修复。现有input_override不得静默删除。

### F02 · P0：查询投影把本地终态当成执行权威

证据：`service/job/service.go:854`的Get只对本地非终态同步；List（905）也跳过终态；GetStepRun（843）仅读库。reconcileConductor（1162）、`execution_projection.go:63`及审批投影均在本地IsTerminal时停止。`generation_result.go`再把Job.RetryCount作为快照有效性条件。

影响：Conductor已原生retry为RUNNING，本地failed不会刷新；当前实现只能依赖“先手动改本地running”解锁查询。暂停被映射成need_confirm（service.go:1326）也把平台展示状态与Human等待混为一谈。

整改边界：引擎状态/精确Task/循环轮次决定读模型；读模型自身不能成为刷新前置条件。取消意图与展示缓存分离；保留精确Task/结果关联的旧消息保护，不保留本地重试周期作为引擎控制条件。

### F03 · P0：Worker代理被本地failed阻塞，并用队列回调修补

证据：`worker_gateway/flow.go:110`和`managed.go`先按本地Job终态拒绝重交付，新poll见failed则调用`retry_projection.go`；该文件GET原实例后更新原Task为IN_PROGRESS/callback=2，直到本地Job解除failed。`flow.go:530`的结果/heartbeat资格仍看本地Job终态。

影响：原生retry产生的合法Task无法正常交付，必须不断回调延迟。回调是此前双权威的补偿，不是业务需求。

整改边界：保留Worker身份、企业/归属、Capability版本和原生Task身份校验；执行资格依原生Task/Workflow事实，不等待本地缓存解锁。普通Heartbeat属于合法代理；为本地Retry缓存反复延期的特殊逻辑应撤出。

### F04 · P0：资源登记依赖用户Get/List，而Finalize输出并非Materials真实ID

证据：`finalize_task/handler.go:62`只按Job/Step/candidate散列出`result_...`并返回；不调用Materials。实际登记在`service.go:943`的finalizeStepRun，由reconcileConductor的finalize分支（1278）调用；Get/List/ownsJob链路会触发它。生成结果单测也通过Svc.Get触发登记。Internal Worker注册的Finalize Handler是这个纯函数。

影响：引擎COMPLETED不等于业务产物已经登记；用户不查询则登记不会发生。Conductor输出ID与数据库Generated Asset ID存在两套身份；多轮生成返回相同内容时Finalize散列也不表示独立生成轮身份。这是执行链路缺口，不能靠更复杂GET事务掩盖。

整改边界：资源登记属于由Conductor调度的Finalize业务Worker，登记成功后才回写Task COMPLETED，返回实际稳定Generated Asset ID；业务写入保留幂等。查询只读/更新可重建投影，不生成业务资源或成为下游推进条件。

### F05 · P1：所有Worker错误被代理强制标为不可重试失败

证据：`worker_gateway/flow.go:549`的SubmitReceipt对所有fail都回写FAILED_WITH_TERMINAL_ERROR；`conductor/runner.go:73`对所有内部Handler错误也如此。与此同时publication/flow.go为Render/Finalize注册retry_count=2，也把Capability RetryPolicy编进Task Definition。

影响：网络错误、服务暂不可用与永久参数错误未区分，已配置的Conductor自动重试策略无法按预期处理暂时错误。

整改边界：明确可重试/永久失败契约，映射到原生FAILED/FAILED_WITH_TERMINAL_ERROR；由Conductor决定何时和是否重试。不能再添加本地Provider执行重试器。

### F06 · P0：旧本地领取/完成/失败入口仍注册，旧调度器仍启动

证据：IDL的ClaimStepRun/CompleteStepRun/FailStepRun仍有生成路由；`router/job/middleware.go`相应钩子为空。handler直接调用Svc.Claim/Complete/Fail；ClaimAtomic（`dal/repo/job/job.go:233`）从本地pending池领取，无owner/企业过滤；Complete/Fail handler不核验目标归属。普通User JWT分支放行，只有Agent分支特别拒绝这些旧路径。Traefik的普通workflow路由包括全部`/api/v1/step-runs`。

`service.go:1027`的advanceDAG、checkJobCompletion、resetDownstream仍在旧路径；main.go:246无条件启动StartClaimTimeoutScanner，ReclaimStale把本地claimed改pending，即使它是Conductor投影。

影响：不是仅“无害保留旧代码”：路由与扫描器仍可修改本地执行事实，且代码存在跨归属操作风险。此结论来自接线/SQL/授权静态证据，未对用户或他人数据实施越权请求，不宣称完成实际攻击验证。

整改边界：退役旧领取/完成/失败接口和扫描器，原生执行只走Worker代理；清理本地DAG执行器及备用无引擎模式。不删除历史业务记录，不把历史导入恢复一起删掉。

### F07 · P1：审批本身已用HUMAN/DO_WHILE，但仍由本地状态/周期预判资格

证据：编译器已生成HUMAN/SWITCH和DO_WHILE。当前公开Confirm/Regenerate handler调用DecideReview，精确Task ID投递值得保留；但`review.go:23`、`approval.go`、`dal/repo/job/review.go:AcceptReview`以本地need_confirm/Job终态/Job.RetryCount接受或拒绝。AcceptReview在Conductor接受前将Step改claimed并重算Job状态；`review_delivery/flow.go:46`也按本地周期否决决定。

影响：原生Human已等待，但未先GetJob刷新本地时可能无法审批；审批收据、展示覆盖与执行状态混在一起。不能因为要简化执行就丢掉决定去重，也不能把这套本地周期继续当成执行机制。

整改边界：用当前精确Human Task及其输入候选/原生轮次验证决定；保留不可变决定审计和最小投递回执。pending表示代理尚未完成交付，不表示已进入下一轮。生成循环由Conductor运行；不额外修改Step attempt来启动它。

### F08 · P1：代理租约、失败交付与原生Task时钟的边界未统一

证据：代理默认独立lease=90秒；Conductor Task的response timeout来自Capability.TimeoutSeconds，Internal任务另为30秒。代理ExpireLeases仅改数据库；Create在原Task/attempt唯一冲突时，另一实例报冲突，同实例可返回原expired记录。SaveResult主要校验本地lease，SubmitReceipt直接回写原Task，不独立核对当前引擎Task状态。

影响：原生重交付与本地expired可能不同步；授权租约可能比引擎响应时钟长。此处为可从代码推出的风险，不能据此声称每种竞态已经运行复现。

整改边界：记录可以绑定调用凭据/原生Task并去重；它不能再拥有另一套Task重试/领取时钟。过期、重交付和技术attempt以Conductor为准；结果回写只针对仍对应的原生Task。Inbox重放属于传输可靠性，不创建下一次业务生成。

### F09 · P1：历史Resume与普通Submit有不同的启动不明确处理

证据：普通start_delivery把不明确发送保留uncertain，只按精确correlation/input/domain读取；旧StartResume仍注释RETURN_EXISTING保障启动去重，错误时只取correlation ID，DispatchPendingResume释放后可再次StartResume。现运行版本的启动幂等扩展已明确暂缓。

整改边界：历史输入验证、CompileResumeExecutableIR及Restore Worker是业务迁移适配，可保留；不要把它用成普通失败/并行分支恢复器。两个初次启动代理契约应一致，不能假设未实现的上游幂等；本报告记录风险，不修改外部Conductor或恢复历史迁移工作。

### F10 · P1：当前DDD与代码仍有互相冲突的权威描述

证据：execution.md新增了“Conductor权威”段落，但14行仍称业务数据库是权威状态；能力表仍定义“重置允许重试步骤”，旧attempt重派发规则也保留。新原生spec与旧字段/旧测试语义尚未统一。旧retry_isolation_test验证的是拒绝seq旁支截断，不是原生retry保持旁支。

整改边界：先统一术语、职责和验收，再改代码；修改测试使其证明用户要求的原生语义，不只证明旧实现自洽。不能以当前全量单测绿色证明边界正确。

### F11 · P1：Worker输出契约与Finalize输入不一致

证据：`worker_gateway/flow.go:445`接受仅有object_path的candidate；`finalize_task.Candidate`只解码url/content，Finalize会把这种已接受结果判为空。输入侧有Capability media_type/MIME检查，Complete侧只检查候选存在任意url/content/object_path，并未按Capability输出媒体类型校验。

整改边界：编译/Worker契约统一稳定资源引用，Finalize消费同一契约，代理只验证并回写；不得先接受结果再由查询临时补字段。受控上传/存储归资源侧，不能以任意外部URL或fixture现成URL冒充完整生产结果链路。

### F12 · P1：编译前校验有版本与required推导缺口

证据：`service/workflow/validator.go`的tplSnapshots只按Template ID缓存，忽略Version；同一Workflow使用同一Template的不同版本时，后续binding/input_schema检查会共用最后读取的版本。deriveInputSchema将共享入口初始设为required=true，遇任一optional槽位就改false，而不是“任一必需消费方就required”；与Submit必填校验及Semantic Compiler的required槽位规则不一致。

这是应在业务编译核心修正的校验问题，不能推给Conductor运行期，也不需要新执行状态机。已发布Compiler主链快照键实际使用Template ID+Version，值得保留并让前置校验一致。

另需明确两个已有编译约束：validator要求输入步骤列表拓扑有序，Semantic Compiler按Order解析此前步骤；FORK/JOIN按wave增加屏障。它们是编译模型选择，不是运行恢复理由。编辑器位置/数组顺序不应悄悄变成执行依赖；是否允许任意序列化顺序及更细粒度并行，须以DSL约定评审，不在本报告中擅自扩展实现。

## 同时发现的必要业务权限缺口

这些不是Conductor该负责的能力，代理化时不能删掉最终授权：

- Template Update/Archive/Publish/GetVersion入口未传当前caller作资源授权；Service对应方法也没有owner检查。GrantTemplate使用请求中的granted_by作owner判断而非认证caller。`handler/template/template_service.go:42`、`service/template/service.go:122/176/184/218`。静态代码缺口已确认，未进行实际越权操作。
- Template/Workflow的team可见性和grant已存，但canRead仅owner/public；授权查询未消费已存team grant。不能把“能写grant”当成共享权限完成，也不通过Conductor去实现平台企业权限。

## 保留／简化／删除清单

| 类别 | 模块 | 处理边界 |
|:---|:---|:---|
| 保留 | Workflow/Template/Capability、validator、semantic、IR、CompileExecutableIR | 这是业务核心；统一为一条发布/编译主链 |
| 保留 | Metadata注册和不可变版本检查 | 编译产物发布，不是运行调度器 |
| 保留 | Render Worker | 执行一个Conductor任务的纯业务函数，不选下个节点 |
| 保留并完善 | Finalize Worker、Materials关联 | 在原生Task中完成资源登记，不由GET驱动 |
| 保留并简化 | Worker API、身份、Task Domain、输入资源解析 | 安全代理，避免二次调度和缓存failed拦截 |
| 保留并简化 | Review审计/精确Human投递、结果Inbox | API去重/交付状态，不增加引擎执行周期 |
| 保留并简化 | Publish/Submit首次接受与关联记录 | 只代理注册/Start及记录归属，不扩展成运行引擎；不删除已部署审计数据 |
| 保留 | 历史导入、历史发布记录、Restore任务 | 独立业务兼容边界；不做普通并行恢复，本轮不扩展迁移 |
| 改为原生 | RetryJob、失败继续型RetryStepRun | 原生retry；改变输入/rerun语义明确区分 |
| 改为只读投影 | Get/List/GetStepRun、本地状态/候选展示 | 不否决引擎状态，不写资源，不推进执行 |
| 删除/退役 | ClaimAtomic本地任务池、advanceDAG、resetDownstream、checkJobCompletion、claim scanner | 第二套调度器；不是Conductor代理 |
| 删除/退役 | retry_commit、retry_impact及普通retry用的seq恢复规划/缓存窗口回调 | 不再为错误的rerun操作叠加分支恢复 |
| 不重新引入 | 已备份撤出的execution_retries、baseline/witness、retry_delivery、migration024 | 用户否定的第二套执行控制；本次未再操作备份或数据库 |
| 清理冗余 | 旧compiler.Compile路径、旧Confirm/Regenerate旁路、未消费的生成元字段 | 先确认引用与历史兼容，不连带删除真实业务能力 |

## 全入口覆盖矩阵

`covered`只表示审查已定位能力及代码；目标实现缺口明确用`missing`，不能把审查完成说成开发完成。

| 原能力/入口（全部RPC） | Domain/事实 | 应参与的Workflow/原生能力 | 当前目标覆盖 |
|:---|:---|:---|:---|
| Workflow Create/Update/Get/List/Archive/Validate/Grant/Revoke/ChangeVisibility | 定义/权限 | 编译前业务模型 | covered；team授权消费missing |
| PublishWorkflow | Compilation/发布回执 | semantic→IR→backend→metadata代理 | covered |
| Template Create/Update/Get/GetVersion/List/Archive/Publish/Grant/Revoke/ChangeVisibility | 模板版本/权限 | 编译输入 | covered；资源写授权missing |
| SubmitJob | 归属/输入/执行关联/提交回执 | 原生Start代理 | covered，启动不明确限制已记录 |
| GetJob/ListJobs/GetStepRun | 执行读模型 | GET原生Workflow/Task | missing（F02/F04） |
| CancelJob | 用户取消意图/执行关联 | 原生terminate | covered的代理骨架；意图与投影分离missing |
| RetryJob/RetryStepRun | 用户操作审计 | 原生失败retry；显式重新开始另分 | missing（F01） |
| ConfirmStepRun/RegenerateStepRun | 精确Human决定/审计 | 原生Human complete、DO_WHILE | 原生结构covered；边界收敛missing（F07） |
| ClaimStepRun/CompleteStepRun/FailStepRun | 旧本地执行状态 | 不应存在的第二套调度入口 | deprecated-with-reason；实际退役missing（F06） |
| RegisterWorker | 执行Actor/能力声明 | 代理身份/契约注册 | covered；并发声明本身不代表服务端调度保障 |
| PollWorkerTask/HeartbeatWorkerTask | Task身份/凭据映射 | 原生poll/update IN_PROGRESS | 原生代理covered；缓存与租约冲突missing（F03/F08） |
| CompleteWorkerTask/FailWorkerTask | 结果回执/业务输出 | 原生Task complete/fail | 原生代理covered；失败分类/结果契约missing（F05） |
| ImportIncompleteExecution/ImportCompletedExecution | 已有媒体与历史来源 | 只登记事实，不启动执行 | covered，external-owner原始历史系统/Materials |
| ResumeJob | 历史输入与启动关联 | CompileResume→原生Start/Restore | covered的业务适配；启动风险见F09，本轮不扩展历史 |
| ImportHistoricalVideoPush/GetHistoricalVideoPush/ListHistoricalVideoPushes | 独立历史发布关联 | 不编入生成Workflow | covered |
| Health | 服务可用性 | 非业务执行 | covered |

### 后台入口覆盖

| 入口 | 分类与审查结论 |
|:---|:---|
| publication.Delivery | 编译注册命令交付，可保留最小投递，不解释运行DAG |
| start_delivery.Flow | 初次Start/关联及取消清理，可保留命令代理边界；不升级为引擎幂等扩展 |
| review_delivery.Flow | Human决定投递，可保留，去除本地执行周期控制 |
| DispatchPendingResume | 历史初次启动适配；风险F09，不复用于普通Retry |
| Worker RunResultSubmitter | 结果回写代理；保留去重，修正原生失败分类与旧Task处理 |
| Worker RunLeaseReaper | 本地授权记录期限不应再是另一套Task重试时钟 |
| StartClaimTimeoutScanner | 正在启动的旧调度器，应退役 |
| Internal Runner | Render/Finalize/Restore的原生Worker；失败分类和Finalize登记需完善 |
| ManagedRuntime.Run | 配置绑定能力的原生任务消费者；不参与DAG调度，交付边界同Personal |
| media_worker.runCapability/heartbeat/submitResult | 网关客户端/传输重试；不执行Render，不解释Workflow；当前只有fixture executor，不冒充真实Provider实现 |

## 已有证据及其限制

- `TestConductorNativeRetryCompiledParallelWorkflow`在实际Conductor 3.32.4上，使用当前编译器图，通过原生retry保留已完成旁支和已渲染提示词、继续失败invoke、JOIN合流完成。初次正确观察36.13s PASS，正式Client adapter复测7.62s PASS。仅SIMPLE队列别名隔离，不改图/表达式；Provider为技术夹具，无业务DB/真实模型。它证明原生能力可用，**不证明公开Job/Worker API已经接入**。
- 两次早期空poll误判后来确认是仍RUNNING/JOIN异步推进，不是Conductor缺失能力；不能将这些失败当成增加补偿层的理由。
- Workflow全量单测/架构检查已进行只读回归；绿色主要证明现有实现一致，不能替代上述边界整改的验收。
- 未对现有用户任务实施重试、取消或越权试验；未启动真实Provider、修改外部Conductor、改UI、迁移历史或改部署。

## 后续实施约束（不是本轮实现）

1. 先统一当前DDD目标及操作语义：失败继续、用户重新生成、改变输入重新开始分别对应原生能力。Step按钮指向所属Workflow继续时，必须明确“原生retry可能恢复多个未成功Task”，不能声称仅目标业务节点局部重跑；若以后要求严格局部重跑，应先用原生子工作流/操作契约表达，再评审编译，不自建恢复器。
2. 退役旧调度入口/扫描器，修正必要业务授权；删除本地控制模块前确认引用并保留历史数据。
3. 将失败继续、查询、Worker交付和审批一起接向原生状态/Task身份；不用缓存状态决定执行资格，不通过更换名字保留原控制器。
4. 把资源登记放进Conductor Finalize任务，修正Worker错误分类和输出契约，验证无查询客户端也能完成执行及资源登记。
5. 再跑授权网关HTTP、独立CLI/客户端Worker的一条完整流程，包含原生失败继续、反馈新轮、多候选审批、成功旁支保留和产物读取。只有这些通过才称可用；不以新增更多孤立补偿测试延后交付。

新增运行代码的审查规则：若代码在决定“哪个节点现在能跑/何时重试/哪个分支被重置/何时Workflow完成”，默认应由Conductor负责；若是验证“谁能访问/如何转换一个Task的参数/如何登记资源/如何安全回写同一Task”，才属于业务Worker或代理边界。

**上述内容为整改前审查基线，目标实现仍有missing，不宣称领域建模整改或开发完成。**

## 2026-10-07 · 用户确认原生语义后的第一批整改

用户明确接受Conductor retry范围；不存在必须实现的单节点局部重跑需求。以下是当前源代码进度，不代表旧运行镜像已经更新：

- F01普通无输入覆盖RetryJob/RetryStepRun已代理原实例retry；删除局部影响规划、renderer定位、分支隔离拒绝与Step手动重置。已RUNNING只查询，结果不明确不盲重发。输入覆盖仍是单独的旧rerun兼容分支，尚未完成其最终边界收敛。
- F02失败/成功缓存不再阻止Get/List/GetStepRun刷新；忽略Conductor retried旧Task，Step技术重试计数取自活动invoke。取消意图保护与旧rerun周期兼容尚在，不将本阶段说成投影完整整改。
- F03Personal/Managed首次交付、重取和活动心跳/结果接受核对原实例及精确活动Task；缓存failed/succeeded不否决引擎RUNNING，不callback延期。代理授权租约及结果Inbox完整交付边界仍属于F08待整改项。
- F06旧claim/complete/fail路由通过中间件返回410，main撤出旧Claim扫描器启动。旧Service/Repository方法和无引擎兼容路径尚需清理；运行数据未删除。
- 更新DDD/spec/plan；旧callback窗口spec/plan标为历史废弃记录，不继续自建Retry控制层。

证据：Workflow全量单测通过；Job/Worker/退役路由/Conductor关键包race通过；架构检查与diff检查通过。实际Conductor3.32.4编译图并行retry38.62s PASS；直接交付技术门禁1.60s PASS、增加原生Task归属核对后复测0.88s PASS。隔离PostgreSQL鉴权门禁35.64s PASS。技术夹具与隔离库不替代公开网关HTTP/独立CLI/客户端Worker完整流程。

补充持久层证据：重试审计不修改执行状态37.12s PASS；包含原生Task归属核对的Personal鉴权复测29.90s PASS；实际Conductor+隔离PostgreSQL的Managed Worker执行回写43.66s PASS。Provider仍为mock，测试只清理自己的数据库容器与技术定义/实例，不证明部署网关或真实Provider。

未部署、未提交；UI、历史迁移、外部Conductor均未修改。Finalize业务登记、Worker错误分类、审批/取消/租约收敛、旧代码完整退役、编译校验及最后部署联调/wiki仍有missing。

## 2026-10-07 · 第二批Finalize整改与部署单样本

- F04：当前内部Finalize不再返回候选散列占位身份。它核对原生Task/本轮唯一成功invoke/候选与审批、读取冻结Job/Step来源，幂等调用Materials后返回真实gen_身份及稳定对象引用。GET/List/GetStep只投影已登记ID，不调用Upsert；Worker不修改Job/Step状态或推进DAG。
- F05内部部分：Runner注入精确任务身份；永久输入/身份/候选错误为FAILED_WITH_TERMINAL_ERROR，临时故障为FAILED，原生Task Definition负责重试。Materials已提交但TCP响应丢失的技术门禁5.17s PASS：编译图无Job查询完成，两次登记同结果键/同Asset。Materials在该技术门禁是HTTP协议替身，持久化另有既有Materials门禁与下述部署流程；Provider错误分类仍未整改。
- 隔离PG投影门禁43.19s PASS：产物引用/步骤状态同事务、失败回滚、晚到审批/取消/旧快照不覆盖；查询全程0登记调用。Workflow全量单测及相关race、架构/diff检查通过。
- 已部署API与内部Worker，镜像标识见plan；没有新迁移，库23/dirty=false。测试账号无待运行执行才提交一个Job，其他账号旧本地记录未修改。CLI dry-run→唯一Submit→独立客户端provider_fixture→HTTP反馈重放→CLI原生retry/选择候选→四节点成功，83.82s PASS。Job `job_eb6c7f61-740b-49b0-9267-df1aabcc090c`，原实例`c2f33315-aadf-4731-84ec-3c3eab9396f9` COMPLETED。3 PNG+1 MP4独立无Token Range206读取4.13s PASS；Materials实际4条资产均有稳定Object Path。未人工修复引擎、重新Submit或执行真实模型，客户端已停止。
- Vite此前无5173监听，代理访问返回502；仅恢复既有开发进程，直接LAN HTTP200，UI代码未改，用户DESIGN.md未动。后端及Vite保持运行；不改外部Conductor、其他外部服务或历史数据。

仍有missing：Provider错误分类、审批/取消/授权租约的完整权威收敛、旧调度器/无引擎路径清理、输入覆盖兼容、编译校验、资源权限缺口、async trace与最终wiki。本阶段证明当前普通四节点mock流程可运行，不宣称Goal或全部审查项完成。

## 2026-10-07 · 用户确认Worker结果直接代理

### 资源权限入口第一阶段

源码补上Template更新事务内owner复核、Archive/Publish/Delete的明确caller，以及公开GetVersionForCaller入口；内部不可变快照GetVersion保留，不由公开handler调用。两个Grant handler从认证上下文取得issuer，忽略请求字段；IDL字段deprecated但不删除编号，DTO/OpenAPI已重新生成。非owner更新/归档/发布返回403；公开私有版本拒绝读取。

service拒绝测试断言零写入/零版本读取；Hertz handler测试通过显式本地principal注入验证query/body伪造owner拒绝、实际owner操作仍通过且Grant审计issuer不能被客户端覆盖。该门禁不冒充真实GatewayJWT、部门成员/IAM或数据库验收。部门Grant读写消费、editor授权、组织边界及授权列表分页仍missing；所有者守卫仅为第一阶段，不把缩窄为owner-only作为最终设计。本批尚未部署，不改用户数据或UI。

### 部署及编译校验后续证据

直接代理版本已停旧进程并核对pending/failed=0后部署，完整网关/独立CLI/客户端单样本33.28s通过；Inbox保持93条、6个Dispatch只记账。详细镜像/Job/原实例与输出读取见直接交付plan。故上文该项“未部署”是部署前阶段记录，不代表当前运行状态。

F12两处实际缺口已修正源代码：快照按Template ID+Version取用，同名入口required为消费方OR。新增测试先在旧代码复现跨版本拒绝正确Slot及optional削弱mandatory的失败，再验证正确版本隔离、错版本拒绝、四种required组合。Workflow全量单测、Workflow/Semantic/Publication race及架构/diff通过；尚未部署新validator，不改已发布快照。已有拓扑顺序/wave契约未扩展，资源权限、审批/取消/租约、旧执行代码/输入覆盖、上传、trace/wiki仍missing。

当前结果回写不再新增Inbox或异步投递，生产SubmitReceipt/RunResultSubmitter已删除；结果先后查询精确原Task，200/submitted才确认，ignored update不能冒充成功。Personal本地私有文件保存/重启恢复发送与executor分离，Managed同次执行内只重发结果；Provider副作用幂等仍需自身保证。旧Inbox仅存档且启动有未确认记录时拒绝切换，初次启动/审批/发布Outbox不变。

证据：真实Conductor自动retry6.14s、实际提交后代理丢响应/原Task查询恢复4.14s，隔离PG鉴权27.27s/Inbox零新增与失败分类22.25s、真实Conductor+PG Managed40.67s通过；Worker真实子进程保存退出后恢复与全量race通过，Workflow全量单测/架构/diff通过。未部署本批，不代替公开网关/独立客户端完整复验。此调整不关闭其他审批/取消/租约/旧代码/编译权限/trace/wiki missing。

## 2026-10-07 · Provider失败分类源代码整改（直接代理前的阶段证据）

此节之后补充的审批整改是独立证据，不代表下方Provider范围已经全部验收。

F05的失败性质映射已接入Personal公开IDL/handler、客户端及Managed executor，transient→FAILED、permanent/旧空分类→FAILED_WITH_TERMINAL_ERROR。旧reason-only JSON保持不变，分类不能通过重放覆盖；未知值在持久化前拒绝。Conductor拥有技术retry调度，没有新的本地Provider重试器。

Workflow全量单测、Worker全量单测及Gateway race通过，架构检查通过。真实Conductor门禁`TestProviderFailureClassificationNativeRetry`3.96s PASS（transient 2.82s，permanent 1.14s）：正常代理交付失败，原生Task Definition自动创建新Task/attempt并完成；永久错误仅一Task且无自动重试。使用唯一技术定义/实例与受控持久端口，不改用户任务或数据库，不冒充公开网关/独立Worker验收。未部署本批代码，F05结果输出/上传契约、F08晚到交付和租约等其余项仍missing。

## 2026-10-07 · 审批执行权威源码收敛

后续Worker超时边界源码已收敛：本地90秒字段不再决定活动结果/心跳资格，精确Task状态、attempt、取消和身份继续校验；Heartbeat提交后再查活动状态。API/Managed本地Reaper与DAL过期扫描删除，旧expired记账可刷新，同身份原生重投保留ID/attempt，不夺取不同绑定身份。Workflow全量、Gateway race/构建/架构通过，真实Conductor过本地时间交付及丢响应4.45s、隔离PG记账/身份/心跳/结果35.83s通过；源码covered，最新运行镜像与公开完整复验missing。上传/输出契约和trace/wiki仍未完成。

后续input_override源码收敛：原生FAILED/TIMED_OUT+精确实例准入、原Compilation冻结输入、POST后GET输入确认；丢响应只由输入/Task身份变化证明，RUNNING不单独作为确认。本地commitRerunProjection仅使查询缓存失效并写输入/审计，不排队原生任务；取消/关联/周期变更拒绝旧提交。Finalize来源优先本轮原生输入，不错误继承旧Job商品/人物。Job/Finalize race通过，隔离PG事务36.55s、真实Conductor+PG丢响应一次POST33.70s通过。简单HUMAN技术图不是完整公开编译Workflow验收，最新代码部署仍missing；不新增执行控制器，不关闭上传/租约/trace/wiki。

后续旧本地调度源码已退役：Service/DAL无ClaimAtomic、ReclaimStale、本地Complete/Fail/旁路Confirm/Regenerate、DAG推进或Claim扫描；Submit缺少注册Compilation/Conductor拒绝而不创建本地任务。旧地址IDL deprecated、Handler直接410，旧CLI本地协议测试换不写资源的网关410门禁。Workflow与CLI全量、关键race、架构/gofmt/diff通过，公开网关旧地址门禁1.01s通过；源码能力covered，最新镜像部署missing。历史数据未删除，旧测试源码由Git可恢复；此记录不关闭输入覆盖、租约、上传或trace/wiki项。

后续取消整改技术门禁已通过：native接口先核对RUNNING/PAUSED、幂等重放取消意图，缺少持久启动记录不回退直接Terminate；后台拒绝不同实例身份。暂停独立paused投影，TERMINATED为cancelled，IDL追加PAUSED=8及迁移24状态约束。实际Conductor+隔离PG取消恢复23.13s、启动事务22.58s、暂停与降级安全40.73s通过（暂停首轮fixture缺实例绑定，修正后通过）；全量/race/架构检查通过。源码技术能力covered，运行库迁移和部署复验missing，以下审批阶段记录保留。

DecideReview改用当前原生HUMAN冻结候选，不以缓存Step状态/候选禁止活动任务；AcceptReview事务只存决定，不提前claim Step或推进Job。精确Task与retried标记用于送达校验，历史accepted_retry_count只保留审计与摘要。送达HTTP成功后仍查询COMPLETED和相同输出；旧缓存失败不否决原生活动Task。已完成HUMAN选择由查询投影保存。

Workflow全量、关键race、archcheck、gofmt/diff通过；隔离PG审计34.80s/事务59.73s，真实Conductor+PG响应丢失31.62s/真实进程重启78.12s通过。技术门禁covered，公开部署复验missing。此批未更新运行镜像；取消、租约、旧调度器/输入覆盖、上传及trace/wiki仍未完成，ACL按用户要求deferred至核心验收后。
