# 2026-10-25 Studio 迁移

P1 经用户确认后接入专用生成实例。增加执行准入、生成检查点、pending/abandoned 恢复语义与确定文件验证；跨领域生成由 generate Workflow 协调。先确认原 Attempt 资格，所有不确定副作用保留原生成 Key，禁止以失败重试或新 Poll 替代恢复。旧结果日志与 fixture 契约保持兼容。

## 2026-10-26 · 执行协议归回技术运行层

用户明确要求移除 execution Domain。Task、Capability、Result 与执行/交付端口移至 biz/shared/worker_runtime；豆包检查点移至 doubao Domain，GPT 独立拥有自身检查点。旧结果日志和豆包检查点的磁盘格式、目录与幂等键保持。
