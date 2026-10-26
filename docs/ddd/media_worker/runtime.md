# Worker 公共运行契约

`execution` 不是业务 Domain。`biz/shared/worker_runtime` 定义网关 Task/Capability、Result、Artifact、执行器、网关、上传器、结果仓储、准入与恢复生命周期接口；这些是客户端运行机制与技术协议，不拥有业务规则或调度权。

`workflow/execute` 编排注册、结果重放、准入、领取、心跳和交付。CLI 负责装配具体 Provider Workflow。`workflow/doubao_generate` 与 `workflow/gpt_generate` 分别实现 Executor 和 ExecutionRecovery；不通过共享 generate 或 Recipe 统一生成行为。

| 契约 | 实现/归属 |
|:---|:---|
| Gateway / Uploader | client/worker_gateway；既有公开协议及授权上传 |
| ResultRepository | dal/repo/results；原 pending/rejected 格式与凭证命名空间 |
| Admission / RecoveryAdmission | 各 Provider adapter 的容量与就绪检查 |
| ExecutionRecovery | 各生成 Workflow 的检查点恢复，不创造执行资格 |
| InputDownloader / ArtifactVerifier | client/media_input；下载与确定文件校验 |

待交付 Result 必须先同步保存再发送；只有网关确认或拒绝后才结束交付。pending 保留原 Attempt 和容量，不产生失败重试；abandoned 表示当前资格已经拒绝。网络 DTO、磁盘 DTO 留在 client/repo，不放在公共运行类型中。

豆包生成意图、引用和 manifest 属于 domain/doubao，其物理检查点仍用旧 namespace/checkpoints 路径；GPT 使用 domain/gpt 和独立 namespace/gpt-checkpoints。公共运行层不定义统一生成状态机或替任一 Provider 解释生成引用。
