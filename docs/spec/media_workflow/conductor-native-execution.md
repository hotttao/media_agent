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

### 内部Finalize与查询投影

`conductor.Runner`为Handler注入只读原生Task身份上下文，Finalize使用现有`job_id/step_key/candidates/selected_candidate/output_asset_kind`输入，不改冻结Definition。加载原Job/Step并核对企业、owner、版本/产物类型；GET原实例，核对精确活动Finalize、当前轮唯一成功invoke及候选。使用`GenerationResultKey(workflow_id, step_run_id, successful_invoke_task_id)`向Materials登记，成功后返回真实`output_resource_id`与`output_resource.id`，媒体引用为企业/Scope命名空间内稳定Object Path。不得以FinalizeTask ID或候选内容散列代替生成身份。

内部Worker使用Materials内部凭据，StorageOrigin使用既有`MEDIA_STORAGE_PUBLIC_ENDPOINT`。对象路径/URL规范化为受控、无签名的URL供现有登记API验证，并把稳定引用返回Conductor供后继领取时重签；纯文本不解析对象。对象类型/大小由Materials存储校验负责。跨企业/Scope、旧候选、错误Task/输出类型或缺少身份拒绝登记。

Get/List/GetStepRun仅读Conductor真实Finalize结果及已有业务引用，事务内投影ID/状态，不调用Materials Upsert。登记失败不能回写Task COMPLETED；登记已提交但回写失败由原生Task重试/重交付恢复同键ID，不新增执行控制层。已有终态Step引用保留；未登记旧占位输出明确拒绝，不能通过查询补登记。

内部错误使用稳定安全码：永久输入/身份/候选错误映射FAILED_WITH_TERMINAL_ERROR，临时外部不可用映射FAILED；Runner不自行执行任务重试。HTTP结果未知时同键Materials登记可以安全重放，不能换生成键。

测试必须证明：无用户查询也能完成Finalize和原生Workflow；登记已提交但响应丢失→原生自动retry返回同一Asset；Finalize新Task ID不改变结果键，新invoke新轮产生新Asset；失败/跨归属/旧轮候选不登记；GET无登记调用。隔离测试不得启动真实Provider或改既有用户执行。

旧ClaimStepRun/CompleteStepRun/FailStepRun路由在扩展中间件返回410，不进入本地任务池Handler；不改变审批、重试或新Worker Gateway路由。main不启动本地Claim超时扫描器。剩余旧Service/Repository实现及IDL/CLI清理仍单独跟踪，不通过删除历史数据完成退役。

实际编译器IR生成FORK_JOIN/DO_WHILE/SWITCH：完成旁支B、A调用失败、原生retry只继续A调用、保留A已渲染提示词与B完整Task身份/结果、JOIN异步推进、合流完成。空bounded poll不等于失败，不更换测试实例。仅别名化SIMPLE Task队列避免干扰实际Worker，不改图/表达式。随后需公开Gateway HTTP、独立CLI与真实Worker的一条完整业务流程；技术探针不替代部署验收。UI/全量历史/Agent不在本轮范围；启动幂等扩展仍暂缓。
