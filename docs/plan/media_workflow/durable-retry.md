# 持久执行重试落地

> 后续P3/P4接线不再执行，状态为deprecated-with-reason：重复引擎执行权威，与用户明确边界冲突。既有测试记录保留，不冒充业务交付。当前计划见[原生执行边界](conductor-native-execution.md)。

依据[spec](../../spec/media_workflow/durable-retry.md)。保留现有Capability和投影窗口补偿，UI/历史/Agent不实施；启动引擎扩展暂缓不撤销。

- [x] P1：值对象、migration024、调用方事务内命令接受/查询、完整作用域/周期/冻结计划/请求重放及隔离PG审计门禁。只存储，不部署接线或宣称Retry可靠完成。
- [x] P2：真实Conductor Step/Job Rerun标识与变化证据契约；无证据不盲发。依该证据实现独立投递/恢复端口、租约及协调器；尚未装配生产循环/API。
  - [x] P2a：持久领取/发送标记/退避释放，数据库租约fencing与发送前并发取消门禁；不代表发送后的取消清理完成。
  - [x] P2b：冻结可核对的摘要化Task基线、引擎变化证据、独立投递和只读恢复；不能用正常Rerun协议测试替代崩溃恢复。
    - [x] 摘要值对象/adapter字段/024基线存储、证据确认与租约门禁。
    - [x] 协调器单测与真实引擎/隔离PG恢复门禁；只读重入不POST，半写入证据不确认。真实进程退出和引擎中间写入故障仍在P3/P4，不由模拟端口冒充。
- [ ] P3：命令应用与Job/Step周期原子提交、查询投影/Worker/审批fencing、取消/晚到及真实进程恢复；替换直接Rerun路径、IDL/handler/CLI回执接线。
- [ ] P4：部署、完整队列审计后无人工单流程HTTP/独立CLI、故障回归及与并行恢复的组合门禁；不以串行正常流程证明并行或崩溃恢复。

覆盖矩阵：普通Retry/本地原子提交及暂时投影窗口covered；持久命令接受/隔离PG审计/投递租约/摘要基线/模拟丢响应后的只读确认covered；实际引擎正常重跑标识与完整任务变化covered；确认后的业务应用、发送后取消/晚到、真实进程恢复、生产循环/回执接线及公开接口故障门禁missing，按P3–P4补齐。引擎多次写入自身原子性为external-owner（Conductor 3.32.4），本地仅完整变化才确认，部分证据等待、不自动重发；实际中间故障仍需门禁。并行旁支恢复missing，仍须原独立设计与隔离门禁；历史执行普通Retry为deprecated-with-reason（使用Resume）；启动引擎幂等扩展deprecated-with-reason（用户暂缓）。有missing不宣称整体建模或Goal完成。

## 2026-10-07 门禁证据

- `TestExecutionRetryCommandPersistence`：27.57s PASS，实际PostgreSQL 16隔离容器从头应用migration001–024。八路同键只建一条；八路同周期不同键仅一条成功；回滚/无事务、旧周期/取消/版本/hash/实例/Step归属与集合、当前归属权限、旧回执重放、冻结字段/证据/终态与降级审计保护通过。容器已清理；用户数据库未应用024。
- `TestConductorRerunCommandMarkerContract`：实际运行中的Conductor 3.32.4，独立新建实例；Step Rerun携完整input后保留命令标识、已完成前置Task及重置目标；再次失败后Job Rerun写新标识并替换旧Task身份，最终COMPLETED仍保留标识。首次wire probe 7.54s PASS，随后正式Client方法复测3.27s PASS。只处理本测试创建的实例/定义。
- 此正常Rerun协议探针不证明workflow input写入与任务重置原子性，更不证明丢响应/真实进程退出的恢复；不据此接入自动重发。之后的独立投递门禁见下；P3/P4仍未完成。
- `TestExecutionRetryDeliveryLease`：28.47s PASS，隔离PostgreSQL从头迁移024。八路领取只有一个owner；发送标记仅caller Tx且仅一次；uncertain重新领取仍拒绝再次标发送；十秒退避、过期/新owner替换后的旧token拒绝、发送事务回滚、发送前取消/Job及Step周期/owner/版本/hash/实例变化拒绝、取消先提交时等待的发送方拒绝，均通过。另以旧事务开始后跨越租约过期线验证`clock_timestamp()`而非事务开始时间。只有存储租约代码，尚无后台循环或生产Retry API接线；用户数据库未迁移。
- 基线必填后的回归：`TestExecutionRetryCommandPersistence` 27.21s PASS、`TestExecutionRetryDeliveryLease` 25.56s PASS、`TestRetryConfirmationTransactionAndFencing` 21.14s PASS。确认的状态/证据同事务、无事务拒绝、回滚仍uncertain、同确认不重写、过期/替换token、取消/周期/归属/hash/实例变化和错误命令/摘要/仅标识均拒绝。原错实例夹具先因新基线一致性校验失败，已同步冻结夹具并重新通过，不作为运行时故障掩盖。
- `TestRetryDeliveryRealConductorPersistence` 最终`-race` 26.55s PASS（含隔离库初始化）：实际Conductor 3.32.4和PG，节点/整单Rerun成功后模拟丢响应仍GET核对并持久engine_confirmed；原uncertain租约过期重新领取只GET、零新增Rerun。持久确认原回执重放不POST，Job仍failed/RetryCount0，不冒充业务应用。独立实例/定义和PG均已清理；最后一次包含完整Definition和精确JSON整数解析的adapter。
- Domain/协调器/adapter `-race` 与Workflow全量单测、architecture check通过。新增歧义seq用例曾发现证据校验缺口，已补当前目标seq唯一和新增前置任务拒绝并重新通过；旧起止时间、旧下游残留、输入/定义/路由错配、仅标识不确认。确认提交失败后的再进入只读收敛由协调器单测覆盖；真实进程退出仍missing。migration024仍未部署（运行schema23/dirty=false），当前HTTP仍是旧直接Rerun；UI未改。
