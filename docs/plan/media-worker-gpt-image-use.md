# GPT 图片 Worker 落地计划

对应 [spec](../spec/media-worker-gpt-image-use.md)。

- [x] 独立 GPT domain/workflow/检查点，保留原 Doubao 实现。
- [x] image-use web 持久化桥接与浏览器准入、受限命令、确切会话恢复。
- [x] 共享配置生成、生产 CLI profile、公开能力 migration。
- [x] 单测/race/vet/架构检查、运行说明和真实环境准备。
- [x] 已登录 Chrome 实际成图与公开网关验收（与技术门禁分别报告）。

## 实现交接（2026-10-26）

用户明确：两套生成流程独立；execution 不应作为业务 Domain。已移除 domain/execution，技术契约归 shared/worker_runtime；原 generate 与仓储正名为 doubao_generate / doubao_checkpoints，原物理检查点目录及格式保持。GPT 独立实现，不留 Recipe、通用 generation Domain 或兼容 alias。

Worker 全包单测/race/vet/build、模块内 archcheck/deadcheck 均通过；Python bridge 9 项行为测试通过。服务端能力 Domain、Semantic Compiler、Publication、Capability/Template service 回归通过。真实 Go client → Python bridge → 固定上游脚本的只读探测通过，浏览器未就绪时拒绝准入。

固定 image-use 已安装于用户私密工具目录；当前实际检测为 chrome_use_installed=false、current_chrome_connected=false。没有执行真实成图、消费生产队列或创建测试 Job。服务端 migration 000028 已提供但尚未应用到运行数据库；真实验收需先连接 chrome-use CLI/扩展，再部署能力契约及服务，最后使用独立 Worker 与测试任务验收。

产品详情页上一轮修改已经单独提交为 `336427b`。本轮结构更正与 GPT 接入按用户要求一并提交；真实 Chrome 与公开网关验收仍未完成。完整边界与验证范围见 [review](../review/media_worker/workflow/0003-gpt_generate/gpt_generate.md)。

## 继续开发（2026-10-27 · 扩展连接前）

- 修复准入在空队列轮询时反复导航 ChatGPT：正常轮询改为只读，草稿或生成中拒绝新任务；增加 5 秒就绪缓存及生成调用失效。
- Python bridge 11 项测试通过，新增真实子进程在写入意图后退出的无重发验证；行为测试已纳入 Go e2e 测试入口。Worker 全包 race 再次通过。
- 在独立临时 PostgreSQL 数据库顺序执行全部 28 个 up migration，通过；确认 GPT 两个契约及参考图上限 4。down 000028 对已发布版本正确拒绝删除；临时库已移除。运行数据库仍为 27，未部署新能力。
- 已构建 Workflow 和 internal_worker 新二进制。运行环境存在执行中 Job，暂未替换服务。
- 已安装并校验 chrome-use v1.5.266，注册本机 native-messaging host；当前 relay 尚未连接，等待用户在当前 Chrome 添加扩展。尚未发送真实 GPT 请求或消费生产队列。

## 真实验收（2026-10-27 · 扩展连接后）

当前 Chrome relay 已连接；Workflow 与 internal_worker 已更新，运行库迁移至 28，两个 GPT 能力 published。专用 Worker 身份 `gpt-image-use-20261027` 串行执行两项能力，凭据仅保存在本机私密目录，有效至 2026-10-28 23:00（北京时间）。原即梦 Job 保持等待原生任务，未取消或重排队。

| 样本 | Job | 结论 |
|:---|:---|:---|
| 初次公开文生图 | `job_77877ced-5a13-4e02-8a5a-d9321a2453d3` | succeeded；绑定超时后人工核验原会话再回收，不计入全自动通过 |
| 一张参考图生图 | `job_d06d0ecd-3012-4f9a-9840-e1b1c5855f1d` | 全自动 succeeded；原文生图结果作输入，发送后约 12 秒自动绑定 |
| 修复后文生图回归 | `job_9657a0d7-d41b-4a9f-88cd-dac81ddd8c2f` | 全自动 succeeded，无人工绑定或重发 |

三个原生实例均 COMPLETED，每个仅一个 GPT 生成 Task、retryCount=0；检查点 closed、单产物已上传。三个产物均通过不携带 API Token 的 Storage Range 206、完整下载及 SHA-256 对比。多参考图的 1～4 张契约、顺序与拒绝超限仍由技术测试覆盖；真实网页样本使用一张参考图。

实际联调修复了新版无 form/main 页面、绑定等待、上游 recover 关闭页面、同图跨消息重复引用等问题。早期诊断的未绑定意图保留原记录，不将其解释为未发送，不重发原意图。新增 Send 确认回执与摘要探测后，最后两项全自动样本均有自动绑定、无人工补绑记录。Python bridge 14 项测试、Worker race/vet/archcheck/deadcheck 通过。详情见 [review](../review/media_worker/workflow/0003-gpt_generate/gpt_generate.md)。
