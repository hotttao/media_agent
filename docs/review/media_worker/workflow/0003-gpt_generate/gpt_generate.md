# 独立 GPT 成图流程实现审查

2026-10-27 更新。按用户边界：GPT 与豆包流程独立；execution 不作为 Domain；generate 正名为 doubao_generate。

## 当前实现映射

| 职责 | 位置 | 状态 |
|:---|:---|:---|
| 公共执行/交付技术协议 | shared/worker_runtime + workflow/execute | covered |
| 豆包生成与检查点 | domain/doubao、workflow/doubao_generate、repo/doubao_checkpoints | covered；保留原目录/格式 |
| GPT 请求、引用与检查点 | domain/gpt、workflow/gpt_generate、repo/gpt_checkpoints | covered；独立状态机 |
| GPT 浏览器协议 | client/gpt/image_use + tools/image_use_bridge.py | covered；严格 web/relay |
| 配置与启动 | config.proto → pb-config，cmd/media_worker/gpt.go | covered |
| 能力发布 | media_workflow migration 000028；多图 max_items 有限上限 | covered；已部署，运行库 28 |
| 当前 Chrome 与首次扩展连接 | chrome-use 官方工具及用户浏览器 | external-owner；已连接并真实成图 |

无通用生成 Recipe、generation Domain、旧包 alias。只有运行循环、上传/结果交付与文件工具共用，两套生成端口与检查点互不依赖。历史 P1 review 保留当时状态，并明确标记新结构。

## 审查后的关键修正

- 上游 CLI 没有持久化任务接口，不能把失败/超时当作未提交。bridge 的请求/启动意图同步后才触发浏览器；不确定发送保留人工状态，已知绑定只 recover。
- 不能只保存 URL 后下载“最后一张图”：同时保存确切 user turn ID 和实际提交文本摘要，恢复前后核验；出现人工追加消息或页面变化不采用新结果。
- 未确认 Send 或发送前错误不能把浏览器当前任意会话绑定为本任务，只有上游确认的单次 Send 且空白会话→单一用户 turn 才可绑定。
- 当前 Chrome 全局串行，原上游跨工具锁必须实际生效；无多账号调度、无 Codex fallback、无自动挑战处理。
- 子进程 stdin 传递私密请求，过滤网关环境凭证，stderr 丢弃，stdout 只接受限量协议 JSON；图片需通过受控路径/私有文件/MIME/大小/SHA 校验。
- CLI 运行期间无法确认执行资格则取消自己的进程组；远端是否已经生成仍以原会话事实恢复，不能因取消重发。
- 新版编辑器不再位于 form、对话不再位于 main。适配只扩展同一 composer/turn 范围，仍排除其他会话图片通知；固定上游源码及 SHA 不变。
- 绑定初始等待过短，不能把绑定失败误判为未发送。Send 确认后保存回执与摘要探测，最多等待 120 秒；仍无确切绑定时保留人工状态。
- 上游 recover 会关闭页面，无法满足下载后身份校验。改用其只读图片查询和下载函数，保留页面；同 file_id 的消息重复引用去重，多个不同文件进入人工状态。

## 验证与限制

已通过：Worker 全包单测与 race（包含原豆包跨进程恢复）、vet、构建、架构与死代码检查；Python bridge 14 项测试，已纳入 Go 测试入口。新增覆盖进程在写入意图后退出、空闲轮询不导航、延迟/失败回读后只绑定一次、恢复保留页面与同文件去重、多产物拒绝。能力 Domain/语义编译/发布/Capability service/Template service 回归此前通过；全部 28 个 up migration 在独立临时库通过，down 000028 正确拒绝删除已发布版本。

当前 Chrome 实际连接后，已部署 Workflow/internal_worker 与 migration 28。专用 Worker 使用公开 API 签发的独立身份，用户接口沿用浏览器正常登录，不导出登录凭据。当前 Worker 同时轮询两个 GPT 能力，测试凭据有效至 2026-10-28 23:00（北京时间）。

| 样本 | 原生实例 | 结果 |
|:---|:---|:---|
| 初次公开文生图 `job_77877ced-5a13-4e02-8a5a-d9321a2453d3` | `624ab586-2271-4961-889d-27d791277722` | COMPLETED；绑定超时，人工核验原会话后回收；不计为全自动 |
| 参考图 `job_d06d0ecd-3012-4f9a-9840-e1b1c5855f1d` | `526c7f61-774a-456d-bf79-0cad4d19fa5c` | 全自动 COMPLETED；一张有序参考图、一次确认发送、自动绑定、上传及闭合检查点 |
| 文生图回归 `job_9657a0d7-d41b-4a9f-88cd-dac81ddd8c2f` | `ab374bb1-310a-4e63-9d6e-0c8a0ea70e89` | 全自动 COMPLETED；无人工补绑或重发 |

三个原生实例各只有一个 GPT 生成 Task 且 retryCount=0。产物分别为 `gen_ee8f2bb2-564c-4edd-8fcd-185f4b90bc01`、`gen_9bb57df2-ab20-4eee-b626-575bf4026a02`、`gen_429f7543-12ed-415b-8646-fd3a11e701ef`。均通过无 API Token 的 Storage Range 206、完整下载及与 Worker SHA-256 的一致性校验。未取消、重试或重排队原即梦等待任务。

真实参考图样本覆盖一张图片；2～4 张顺序和拒绝第 5 张由技术测试覆盖，未宣称全部经过网页实测。早期诊断留下的未绑定意图保留人工状态，不将缺失绑定当作未发送。真实进程中断门禁不替代每一种浏览器故障的实际演练；上游 DOM 或 adapter 变化仍可能要求重新适配。

本机临时 Go/C 工具链不进入仓库。固定第三方脚本安装在用户工具目录，上游 commit/SHA 写入 bridge，升级必须重新审查适配；不将完整第三方源码、浏览器状态、Token 或生成媒体提交。
