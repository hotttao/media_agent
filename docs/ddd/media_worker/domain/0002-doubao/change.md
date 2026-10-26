# 2026-10-25 Studio 迁移

P1 经用户确认后接入专用生成实例。增加执行准入、生成检查点、pending/abandoned 恢复语义与确定文件验证；跨领域生成由 generate Workflow 协调。先确认原 Attempt 资格，所有不确定副作用保留原生成 Key，禁止以失败重试或新 Poll 替代恢复。旧结果日志与 fixture 契约保持兼容。

## 2026-10-26 · 独立豆包生成边界

生成工作流命名为 doubao_generate；接回自身检查点事实与仓储接口，不与 GPT 生成流程混用。
