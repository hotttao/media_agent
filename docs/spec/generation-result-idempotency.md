# 原生生成结果登记规格

## 范围与身份

每轮被接受的能力结果对应独立不可变 Generated Asset。StepRun 是稳定节点，output_asset_id 表示当前选定结果。历史资源原样保留，不回填虚构结果键；不涉及 UI、旧数据迁移或模型执行质量。

`result_key` 为 SHA-256，输入是规范 JSON 数组 `[ConductorWorkflowID, StepRunID, successfulCapabilityTaskID]`。同一轮只接受唯一 COMPLETED Capability task；歧义或缺失时不得登记。不能用 Finalize Task ID、Worker dispatch attempt、图片 checksum 或动态签名 URL 作为结果身份。失败后新成功能力 Task ID 不同，重新生成轮次也不同；Finalize 自动重试和网络重放不改变此键。

## Materials 持久化与接口

内部 UpsertGeneratedAssetReq 增加字段15 `result_key`，仅可信服务调用；Workflow 客户端始终提供。新增 `media_materials.generated_asset_results`：step_run_id、result_key、asset_id、request_digest、created_at；主键(step_run_id,result_key)、asset_id 唯一且同库 FK RESTRICT。键/摘要均为小写64位十六进制。非空回执表禁止自动降级删除；不增加跨服务 FK。

登记先验证来源企业、Scope、owner、Template，并规范化媒体为 Object Path。请求摘要覆盖企业/Scope/kind/name/owner/visibility/team/Job/StepRun/Template版本/Object Path/content/来源Product与VirtualIP；不含短期 URL 或系统创建时间。同键同摘要直接返回原资源；同键不同摘要409。新键才验证对象当前可读，并在调用方本地事务内锁定该键、重查回执、创建 Generated Asset 与回执。不得在 DB 事务内请求 Storage。并发首次登记只创建一个 Asset，任意保存失败一起回滚。

同键重放保留原资源 ID，包括进程重启和已失效签名；软删除不能让相同结果键产生新 ID，返回不可用/冲突而非重建。原生结果名称由稳定节点名与完整结果键组合，避免跨轮名称唯一约束碰撞。旧无键请求仅兼容既有行为，不能作为新原生执行的收口路径。

## Workflow 接线与恢复

对当前生成轮次唯一成功 invoke task 派生结果键，Finalizer 输出仍由内部任务执行。调用 Materials 前核对 Execution 快照，在事务外登记；得到 Asset ID 后 Job→Step 锁内再验证当前执行/轮次，原子保存 output_asset_id 与成功状态。取消/Retry 期间晚到登记不关联当前节点，也不删旧资产。成功结果键必须随被接受结果保持稳定，审批决定轮次、成功能力任务身份与执行快照同时核对。

服务故障只返回安全码，不输出 Result payload、签名 URL、上游 body 或凭据。可靠 Retry 意图、注册 Outbox 与 generation 乱序投影仍分别验收，不能以本规格的产物去重替代。
