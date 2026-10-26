# GPT 生成流程变更

## 2026-10-26 · 独立编排

用户要求不与 Studio 共用 generate。新增 gpt_generate，通过公共 Executor/ExecutionRecovery 接入 execute；持久化边界、浏览器绑定、人工与恢复行为由 GPT 自己定义。旧 generate 正名为 doubao_generate。
