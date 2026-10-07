# 重试交付窗口补偿

> 已废弃：用户确认执行语义由Conductor拥有后，缓存failed不再阻塞原生活动Task。callback窗口补偿已撤出，现行契约见[原生执行边界](conductor-native-execution.md)。下文仅保留历史实现记录。

范围：修复已领取任务在本地Retry提交前因Job仍failed被直接遗失的交付问题；不实现新的启动幂等、并行DAG恢复、完整持久Retry意图，不改UI/历史/Agent。

Flow增加原语Execution查询端口，main及Managed Runtime wiring传入现有ExecutionEngine。EngineExecution查询结果带WorkflowInstanceID供严格核对；旧调用方仍保留原读取行为。

新任务poll完成身份、企业、原引擎实例、Step归属和输入schema校验后，对native JobFailed调用延迟交付协调。查询原实例，要求精确实例ID、RUNNING；只能存在一条匹配原Task ID的活动SCHEDULED/IN_PROGRESS任务，TaskType等于注册ConductorTaskDef，完整Input与poll相同，Output为空。不可按reference名称定位。

允许时通过已有CapabilityTaskEngine更新原任务IN_PROGRESS/callbackAfterSeconds=2，保留WorkerID（Personal为服务代理，Managed为配置实例）。不创建Dispatch/Inbox，不发送结果或改变Job/RetryCount，不在DB事务内进行外部调用。成功返回无任务；正常再次poll时按最新Job状态、存储MIME/数量及租约约束交付。

错User/企业/实例在查询前拒绝；取消/成功/legacy_import拒绝重排队。缺查询端口、错实例、引擎终态、任务缺失/重复/类型或输入不一致/有输出都失败关闭；查询或回调错误只返回固定错误，不泄漏上游输入、URL或凭据。引擎超时仍有界；本地Retry无法提交的永久缺口不由本补偿解决。

测试：确定性模拟poll先于本地提交，验证仅原ID回调、0 Dispatch及再次poll可交付；覆盖所有身份/终态/快照错误及Managed隔离。实际Conductor门禁仅创建自身唯一技术SIMPLE定义/实例，以受控Job状态制造窗口，验证真实回调重新领取相同Task ID；不调用模型、不修改用户Job、不以此替代公开HTTP/CLI或进程崩溃验收。
