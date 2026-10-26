# 独立 GPT 成图流程实现审查

2026-10-26。按用户最新边界修正：不把 GPT 兼容进豆包流程；移除 execution Domain；generate 正名为 doubao_generate。

## 当前实现映射

| 职责 | 位置 | 状态 |
|:---|:---|:---|
| 公共执行/交付技术协议 | shared/worker_runtime + workflow/execute | covered |
| 豆包生成与检查点 | domain/doubao、workflow/doubao_generate、repo/doubao_checkpoints | covered；保留原目录/格式 |
| GPT 请求、引用与检查点 | domain/gpt、workflow/gpt_generate、repo/gpt_checkpoints | covered；独立状态机 |
| GPT 浏览器协议 | client/gpt/image_use + tools/image_use_bridge.py | covered；严格 web/relay |
| 配置与启动 | config.proto → pb-config，cmd/media_worker/gpt.go | covered |
| 能力发布 | media_workflow migration 000028；多图 max_items 有限上限 | covered；尚未部署 |
| 当前 Chrome 与首次扩展连接 | chrome-use 官方工具及用户浏览器 | external-owner；尚未连接 |

无通用生成 Recipe、generation Domain、旧包 alias。只有运行循环、上传/结果交付与文件工具共用，两套生成端口与检查点互不依赖。历史 P1 review 保留当时状态，并明确标记新结构。

## 审查后的关键修正

- 上游 CLI 没有持久化任务接口，不能把失败/超时当作未提交。bridge 的请求/启动意图同步后才触发浏览器；不确定发送保留人工状态，已知绑定只 recover。
- 不能只保存 URL 后下载“最后一张图”：同时保存确切 user turn ID 和实际提交文本摘要，恢复前后核验；出现人工追加消息或页面变化不采用新结果。
- 未确认 Send 或发送前错误不能把浏览器当前任意会话绑定为本任务，只有上游确认的单次 Send 且空白会话→单一用户 turn 才可绑定。
- 当前 Chrome 全局串行，原上游跨工具锁必须实际生效；无多账号调度、无 Codex fallback、无自动挑战处理。
- 子进程 stdin 传递私密请求，过滤网关环境凭证，stderr 丢弃，stdout 只接受限量协议 JSON；图片需通过受控路径/私有文件/MIME/大小/SHA 校验。
- CLI 运行期间无法确认执行资格则取消自己的进程组；远端是否已经生成仍以原会话事实恢复，不能因取消重发。

## 验证与限制

已通过：Worker 全包单测与 race（包含原豆包跨进程恢复）、vet、构建、Worker 架构与死代码检查；Python bridge 9 项测试；能力 Domain/语义编译/发布/Capability service/Template service 回归。另用实际 Go adapter 调用实际 Python bridge 与固定 image-use 源码，确认干净台账返回 authoritative absent，浏览器未就绪拒绝 Admission。

真实图像生成和公开网关闭环尚未验收。实际探测：chrome-use CLI 未安装，当前 Chrome relay 未连接；未对原登录浏览器发提示词、未消费生产队列。扩展首次授权需要在当前 Chrome 完成。migration 尚未应用，不能声称已经可以从 Workflow 直接提交 GPT Job。

本机临时 Go/C 工具链不进入仓库。固定第三方脚本安装在用户工具目录，上游 commit/SHA 写入 bridge，升级必须重新审查适配；不将完整第三方源码、浏览器状态、Token 或生成媒体提交。
