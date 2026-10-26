# GPT 图片 Worker 落地计划

对应 [spec](../spec/media-worker-gpt-image-use.md)。

- [x] 独立 GPT domain/workflow/检查点，保留原 Doubao 实现。
- [x] image-use web 持久化桥接与浏览器准入、受限命令、确切会话恢复。
- [x] 共享配置生成、生产 CLI profile、公开能力 migration。
- [x] 单测/race/vet/架构检查、运行说明和真实环境准备。
- [ ] 已登录 Chrome 实际成图与公开网关验收（与技术门禁分别报告）。

## 当前交接（2026-10-26）

用户明确：两套生成流程独立；execution 不应作为业务 Domain。已移除 domain/execution，技术契约归 shared/worker_runtime；原 generate 与仓储正名为 doubao_generate / doubao_checkpoints，原物理检查点目录及格式保持。GPT 独立实现，不留 Recipe、通用 generation Domain 或兼容 alias。

Worker 全包单测/race/vet/build、模块内 archcheck/deadcheck 均通过；Python bridge 9 项行为测试通过。服务端能力 Domain、Semantic Compiler、Publication、Capability/Template service 回归通过。真实 Go client → Python bridge → 固定上游脚本的只读探测通过，浏览器未就绪时拒绝准入。

固定 image-use 已安装于用户私密工具目录；当前实际检测为 chrome_use_installed=false、current_chrome_connected=false。没有执行真实成图、消费生产队列或创建测试 Job。服务端 migration 000028 已提供但尚未应用到运行数据库；真实验收需先连接 chrome-use CLI/扩展，再部署能力契约及服务，最后使用独立 Worker 与测试任务验收。

产品详情页上一轮修改已经单独提交为 `336427b`。本轮结构更正与 GPT 接入按用户要求一并提交；真实 Chrome 与公开网关验收仍未完成。完整边界与验证范围见 [review](../review/media_worker/workflow/0003-gpt_generate/gpt_generate.md)。
