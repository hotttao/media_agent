---
name: gpt
description: ChatGPT 浏览器成图的生成身份、会话绑定与恢复事实
dependencies: []
related_workflow: [gpt_generate]
---

## 职责与边界

拥有 GPT 图片生成请求、引用、图片产物和单次生成检查点。与豆包 Domain 和 Studio 流程无依赖；公共 Task/Result/上传等技术协议由 shared/worker_runtime 定义，不再建立 execution Domain。

## 能力清单

| 能力 | 契约 | 约束 |
|:---|:---|:---|
| 文生图 | gpt.t2i.text_prompt@1 | prompt，零参考图 |
| 参考图成图 | gpt.i2i.multi_image_prompt@1 | prompt，1～4 张有序图片，不截断 |
| 生成器 | Generator.Submit/Lookup/Observe/FetchArtifacts | 固定 web，不回退到 Codex |
| 检查点 | Repository.Save/Load/Pending | 原 Dispatch/Attempt，独立持久化 |

## 聚合与不变量

Checkpoint 为一次 GPT 生成的本地恢复边界，包含不可变的 Request、一次性绑定的 Reference、确定文件与 Object Path；不能改变请求、覆盖引用或重开 closed。请求不含签名 URL、网关 Token。Artifact 由确切 ID 和内容摘要识别，不能按最近生成文件推断。

Reference 是 adapter 的不透明身份，不等于浏览器 URL。adapter 在自己的严格台账中绑定确切 ChatGPT conversation 和 user turn；已有发送意图永不再次调用生成入口。权威不存在来自有效台账；损坏/丢失台账、响应不确定均不能授权重发。

## 模型

| 值/实体 | 字段与含义 |
|:---|:---|
| Request | Key、Prompt、Images、Size、WebModel；网页 size 是提示，不保证像素 |
| LocalImage | Path、ChecksumSHA256、SizeBytes；保留输入顺序 |
| Checkpoint | DispatchID、Attempt、Capability、TraceParent、Request、Reference、Artifacts、Closed |
| UploadedArtifact | ArtifactID、LocalPath、ChecksumSHA256、SizeBytes、ObjectPath |
| Observation | generating / needs_human / succeeded；无不确定转失败重试 |

## 状态与存储

意图 → 原引用 → 单个图片 manifest → 确认 Object Path → closed；写入同步后才进入后续外部调用。dal/repo/gpt_checkpoints 只负责这些事实的原子保存，物理目录 namespace/gpt-checkpoints，与豆包独立。

image-use bridge 的技术台账位于 namespace/image-use，分别记录不可逆启动意图、会话与 turn 绑定、图片完成 manifest。该台账不替代 Workflow 检查点或 Result 日志；三者各自恢复对应边界。

## Workflow 判定

真实浏览器、长时间 CLI、外部媒体下载和网关上传属于跨边界副作用，由 [gpt_generate](../../workflow/0003-gpt_generate/gpt_generate.md) 独立编排。本 Domain 不领取任务、不推进 DAG、不维护账号池。无领域事件。
