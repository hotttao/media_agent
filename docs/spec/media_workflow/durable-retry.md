# 持久执行重试

> 已废弃接线：用户明确要求执行语义归Conductor，不在业务侧再建控制状态机。本规格保留为历史方案，不作为当前目标；替代规格为[原生执行边界](conductor-native-execution.md)。migration024未部署，自建代码撤出，不删除运行数据。

## 范围

实现已存在的RetryJob/RetryStepRun可靠协调，保留最新反馈与冻结DAG，不重复已完成旁支。UI、历史全量、Agent、Conductor启动幂等扩展不涉及。当前直接Rerun→本地commit路径必须在后续接线阶段替换；P1新增持久层时不得宣称运行时已使用命令。

## 持久命令

domain/job定义ExecutionRetry和RetryIdentity、RetryStepSnapshot。身份为id/job/User/企业/Workspace/request_key；kind为job或step。冻结from_retry_count、Workflow ID/version、Compilation hash、原Conductor实例、原因、可选input_override、workflow_input、reset_steps和baseline_digest。Step命令有目标StepRun/Renderer ID，不能覆盖输入；Job命令目标为空且重置集合覆盖全部节点。请求摘要不含后台选出的任务/周期，包含身份、kind/目标Step、原因及覆盖输入；全部命令摘要包含全部冻结字段。JSON摘要规范化，拒绝null输入、非object覆盖、重复Step ID/key、负计数、越界/缺失标识。

Workflow input包含job_id、inputs object、`__media_retry_command_id`=命令ID；资源asset_ref仅空文本引用或稳定Object Path。该标识不是Provider凭据，也不进入Render context的业务槽位。发送前基线为原终态精确实例及全部相关Task身份/输入输出摘要，不能保存完整候选签名URL。

## P1存储

migration024创建media_workflow.execution_retries，字段遵循Execution domain表；无回填。Job与Step外键、Job/key与Job/from_epoch唯一，输入/目标/状态/租约/摘要格式约束，冻结字段不可修改或删除，生命周期不能回退。原生Job不改现有表/IDL状态。表非空降级拒绝。

JobRepo新增EnqueueRetry、GetRetry。Enqueue必须使用调用方TxRunner事务，锁Job，先检查当前归属，再按完整作用域/key读取旧回执；同请求返回原记录，即使当前Job已经不是failed，不重新校验资格或重建旧计划。新命令要求native/failed及原周期、业务版本/hash/实例一致。按Step ID稳定顺序锁重置集合，核对Job归属/key/原计数；Step目标须在集合且failed/skipped；Job集合须全覆盖。DAG影响范围由现有冻结图分析预检，不在Repository重新计算。P1只接受queued，不调用引擎，不改Job/Step计数，不提供未实现的发送/应用方法。

## 后续协调与API

### 摘要化基线和证据

未部署的migration024补充必填baseline jsonb（object、≤1MiB），无生产回填或修改已部署迁移。ExecutionRetry持有RetryBaseline及baseline_digest，摘要同时纳入payload。RetryBaseline：workflow_instance_id、definition_name/version/digest、status、input_digest、task_domains_digest、tasks数组（1–4096），任务快照包含id/type/reference/seq/status/scheduled_time/start_time/end_time/retry_count/input_digest/output_digest。按seq/ID排序，ID唯一，计数时间非负、摘要64位hex。原基线仅FAILED/TIMED_OUT/FAILED_WITH_TERMINAL_ERROR；目标精确Task须存在。EngineExecution补充完整Definition、Input、Task Domain及Task时间/计数，adapter完整解析实际响应，不只取名称版本。nil任务输入输出统一空object计算摘要，不保存Task正文。

RetryWitness包含command_id、payload_digest和observed快照，整体≤1MiB。VerifyWitness检查原实例、定义（完整摘要和name/version）、路由不变及observed输入摘要等于冻结新workflow_input。Step原Renderer仍存在且type/reference/seq一致，scheduled_time增加、目标seq唯一、活动任务end_time为0且scheduled的start_time为0；所有seq更小Task与原快照相同且无新增前置任务，seq更大旧ID都不存在；Job所有旧ID不存在且非空新Task。快照允许RUNNING或合法失败/完成，不以运行状态充当证明。重复ID、字段缺失、不变目标、仅标识、未截断旧下游、前置任务变化、不匹配输入/定义/路由均拒绝。Adapter冻结input及完整Definition用JSON Number解析/传输，不能将大整数四舍五入后认定同一输入；已有Task业务输出读取形态保持兼容。

ConfirmRetry要求调用方事务、当前有效claim、原Job作用域/失败周期/版本/hash/实例，uncertain且未存witness才写engine_confirmed及不可变witness；该能力不应用Job/Step。读回engine_confirmed必须再次验证持久witness。

`biz/workflow/retry_delivery/flow.go`包含Flow和窄TxRunner接口，Engine端口为Get和RerunRetry。DispatchOne返回待应用的已确认命令：queued先GET与baseline比较，caller TX MarkRetrySent提交后才RerunRetry一次，随后GET证据并caller TX ConfirmRetry；响应错误不直接猜失败，ctx可用时继续只读核对。uncertain只GET；engine_confirmed返回原witness，不调用Rerun。错误只用固定码释放租约/退避。Job归属/周期/失败资格变更或取消不POST、不确认，交后续取消/冲突处理。无生产循环、API或应用回执接线时不得宣称可靠Retry完成。

ExecutionRetryRepo新增ClaimRetry/MarkRetrySent/ReleaseRetryClaim。命令读模型携claim_token，但token不参与冻结摘要或对外回执。ClaimRetry原子领取到期queued/uncertain/engine_confirmed/cancel_pending记录，SKIP LOCKED、随机token、数据库两分钟租约并递增领取计数；终态和needs_attention不自动领取。有效期使用`clock_timestamp()`，不以冻结的事务开始时间延长租约。MarkRetrySent必须调用方事务，Job→Step（排序）→命令锁序、复验原native/failed与完整作用域/周期/业务版本/hash/实例/冻结payload及所有重置Step归属/key/计数、目标失败状态、整单全覆盖，当前有效token且queued才能转uncertain一次。失败不授权引擎调用。ReleaseRetryClaim仅有效token可释放到十秒后，错误码只允许小写字母和下划线且长度1–80，不保存原始错误或签名URL。旧/过期token拒绝；领取/标记/释放不改变Job/Step事实。

后台独立租约，queued仅一次标uncertain后发送；uncertain不盲发。Rerun携冻结输入中的命令标识，精确读取原实例核对冻结definition/input/任务变化，证据不足保留不明确状态。必须先证明实际Conductor对Step/Job Rerun标识写入与任务重置的契约，不能只因字段存在就应用。

应用在Job→Step事务内核对旧周期/hash/实例，按已冻结计划重置、追加一次历史、增加周期并同事务标applied。applied重放不再执行计划，取消不复活；未知晚到Rerun的取消清理仍必须单独验证。

公开接口增加稳定key与不透明重试回执；CLI传key、接受异步状态并通过Job查询观察。缺key的兼容调用不承诺跨进程网络重放幂等；同Job周期唯一约束仍防并发重跑。具体IDL在接线阶段更新和生成，当前200同步语义不能被仅有存储代码冒充已迁移。后台确认前保护Job投影、Worker交付和旧审批/结果，不把旧FAILED或旧产物应用到新周期。

## 门禁

P1 domain/Repo隔离PG：同键同义重放、异义冲突、八路同周期竞争、作用域/任务身份拒绝、事务回滚/无事务拒绝、旧周期/取消/版本变更拒绝、冻结与降级审计保护。后续实际Conductor两种Rerun标识、提交后丢响应/发送前后真实进程退出、本地提交失败、取消/晚到结果、公开HTTP/独立CLI及并行旁支恢复必须各自给出证据。
