# Conductor 作为技术编排引擎，业务执行状态仍归 media_workflow

> 2026-09-25：本文“业务 Step 固定映射为通用 `media_step_execution_v1`”的降低方式已被 [ADR 0015](0015-executable-workflow-ir.md) 取代；Conductor 作为执行引擎、业务状态仍归 `media_workflow` 的边界继续有效。

已发布 Workflow 版本编译为不可变的 Conductor Workflow Definition；Conductor 负责持久调度、重试、并行和等待，media_workflow 的 Job、StepRun、Candidate Review 与最终资源 ID 仍是业务事实源。业务 Step 不与一个 Conductor SIMPLE Task 画等号，而是编译为固定版本的 `media_step_execution_v1` 子工作流，内部完成复用判定、Execution Capability 调用、候选持久化、人工等待和最终单输出。

任意合法 DAG 先计算 Topological Wave。单 Step 波次直接发起一个 SUB_WORKFLOW；多 Step 波次使用 `FORK_JOIN_DYNAMIC` 为每个 Step 发起同一子工作流，再由 JOIN 形成波次屏障。该算法保持语义正确，但可能让只依赖上一波部分节点的 Step 多等待；P0 接受该吞吐折损，后续可在不改变领域 Workflow 的前提下优化编译器。

发布时保存 `(workflow_id, workflow_version) -> (conductor_name, conductor_version, definition_hash)`，定义名和版本一经可执行即不原地覆盖。启动 Job 使用 `job_id` 作为 correlation/idempotency key；Worker 结果必须先在 `media_workflow` 幂等落入 Result Inbox，再由适配器确认 Conductor Task，重复投递不得创建重复资源。Conductor 不对浏览器或客户端 Worker 公网开放；客户端通过受 Ory 保护的 Worker Gateway，按 Service Token scope、Organization、Job 所有权和 Capability 授权拉取任务，具体代理边界见 [ADR 0017](0017-worker-gateway-conductor-proxy.md)。

## 状态元数据

accepted-partially-superseded
