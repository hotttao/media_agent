> 历史审查记录。2026-10-26 起 execution 已归回公共技术运行层，generate 更名 doubao_generate；当前结构见 `docs/ddd/media_worker/runtime.md`。

# Execution 结构迁移 review

日期：2026-10-25。范围：[execution 模型](../../../../ddd/media_worker/runtime.md)、[迁移 spec](../../../../spec/media-worker-doubao.md) 的 P1。

## 结论与修正

本批模型与实现一致。Task、Capability、Result、Checkpoint 和五类端口统一在 `media_worker/biz/domain/execution`，无其他业务层依赖、序列化标签、文件或网络访问。HTTP 与磁盘各自定义 DTO，领域 Task 不含 input_json。execution 和 doubao 不相互引用，无兼容转发 alias。

原结果模型附带 HTTP Task，迁移时拆分为领域 Result 和独立 recordDTO/taskDTO，保留原字段名、空 input_json、哈希命名空间及文件名、0700/0600 权限。literal 测试验证旧格式直接读取、同结果重复保存、拒绝归档；e2e 子进程写入旧格式字面量，重启后通过真实 HTTP client 使用原键交付并移除结果。没有清空或转换用户状态目录。

客户端在边界将 409/410 转成 ErrResultRejected，submitted 判定、Bearer、failure_kind 与上传协议保持不变。http 测试覆盖 wire 能力字段、拒绝语义、未确认回执、trace、Token 与上传隔离。

## 能力覆盖矩阵

仅枚举当前实际入口；生产 studio 属于确认后阶段，不算作本批已实现能力。

| 原能力/入口 | 目标上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| CLI profile/env/flag、fixture 结果覆盖/多候选/故障注入 | media_worker | execution | Capability 声明 | execute | covered |
| provider_fixture 配置、按 Job/Step 精确绑定、已上传 URL 或本地文件 | media_worker | execution | Task / Result | execute | covered |
| media_ai_fixture 历史四类媒体与 provenance | media_worker | execution | Task / Result | execute | covered |
| 能力白名单和版本去重 | media_worker | execution | Capability | execute + policy | covered |
| Register / Poll / Heartbeat | media_worker | execution | Task 信封 | execute | covered |
| Complete / FailWithKind / submitted 回执 | media_worker | execution | Result | execute | covered |
| Fail 默认 permanent 便捷入口 | media_worker | execution | Result | execute 的 FailWithKind；client 辅助 API | covered |
| 授权上传、摘要、确定 object_path、PUT 不确定确认 | media_worker | execution | 已确定候选文件 | execute 调用 Executor，入口注入上传 | covered |
| 私有日志 Open/Save/Pending/Finish/进程锁 | media_worker | execution | Result | execute | covered |
| trace 提取、输入剥离、HTTP 与事件关联 | media_worker | execution | Task 元数据 | execute + shared/correlation | covered |
| 服务端租约、调度、候选审批、资源归属 | media_workflow / media_materials | 各服务原领域 | 服务端权威事实 | Worker Gateway / Materials | external-owner |

无 missing。现有 fixture（包括 multi_image 和 jimeng 契约）原样保留测试用途，不声明它们已获真实豆包生产支持。

## 后续准入

CheckpointRepository 本批只有接口；完整生成恢复需保存意图、引用、manifest 和上传事实，且先补齐旧 Attempt 资格确认，不能用重新 Poll 推断资格。生成检查点与待交付日志保持分离。
