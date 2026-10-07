# 重试交付窗口落地

> 已废弃：以下勾选项是旧回调方案的历史证据，不代表当前原生执行接线已部署。现行计划见[原生执行边界收敛](conductor-native-execution.md)，不再继续本计划或添加持久Retry控制层。

依据[spec](../../spec/media_workflow/retry-delivery-window.md)。原用户范围保持不变，UI/历史/Agent不执行；此项只是可靠Retry的一部分。

- [x] P1：Execution查询身份、原任务延迟交付协调、Personal/Managed wiring；补偿不改变业务状态。
- [x] P2：确定性窗口、20组身份/终态/快照不一致/不可用拒绝与回调失败、取消发生于查询期间拒绝、正常再次领取及Managed身份门禁；关键包race通过，Workflow全包/架构通过。
- [x] P3：唯一技术任务的真实Conductor回调门禁；无模型/用户数据变更。`TestRetryProjectionWindowRealConductorRedelivery`真实已运行Conductor通过3.39s，Gateway生产回调后领取相同Task ID、正常完成实例，无人工引擎修复。本地Job为受控端口，不证明数据库事务或崩溃恢复。
- [x] P4：2026-10-07部署API镜像`sha256:e081f5c63dd160040662fc6a5967074608e29be4571d6b881f5c64f607395b63`，库23/dirty=false。重新完整审计261条Job活跃0，CLI dry-run及仅一次Submit，新回归Job`job_79ba8969-bc99-48af-b8aa-bf9bc781e090`。FixtureConfig 4.61s、无继续观察/人工引擎操作的Execution 24.46s通过：HTTP regenerate及重放→第2次model dispatch mock fail→独立CLI Retry→确认数组位置1→四节点成功。3 PNG+1 MP4 Range206、终态回执重放/反向409通过0.87s。回归中重试产生的新Task `9220cb4e-7928-441d-8529-875750714b94` pollCount=2，同Task完成；旧fail Task ID不同，600秒response timeout远未到期，正常Lease也未过期，与自动精确回调重投一致。没有人工release、继续模式或再次Submit，fixture已停止，后端常驻。原失败/人工恢复报告保留。

覆盖矩阵：新任务常规交付covered；已授权任务处于Retry提交窗口的精确延迟交付代码/技术队列/部署公开单流程covered；持久Retry命令及崩溃恢复missing，仍归主计划，不能把本窗口补偿当作整项可靠Retry完成；启动引擎幂等deprecated-with-reason（用户明确暂缓，本轮不实现）。
