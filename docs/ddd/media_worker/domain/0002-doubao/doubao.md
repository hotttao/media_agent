---
name: doubao
description: 可替换媒体生成器的请求与生成事实
dependencies: []
related_workflow:
  - generate
---

## 一句话职责

定义可替换 Generator 的提交、按稳定 Key 查询、观察与精确产物下载契约。

## 边界

### 做什么

拥有生成请求、引用、观察状态和产物身份的统一语言。

### 不做什么

不暴露 CSV、Electron、CDP、studio batch/task，不提供成功占位实现，不负责队列或租约。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 提交意图 | 稳定 Key、已渲染 Prompt、显式模型/时长/比例、有序本地图片 | Generator.Submit |
| 恢复查询 | 确切存在、确切不存在、结果不确定 | Generator.Lookup |
| 观察生成 | 已受理、生成中、需人工、成功、失败 | Generator.Observe |
| 精确下载 | 按请求产物 ID 返回已完成本地文件 | Generator.FetchArtifacts |

## 聚合与不变量

Generator 端口由专用 studio 本机控制实现适配；生成运行事实由 studio 保存，Worker 只持久化其不透明引用。Reference 不透明，Artifact ID 是确切关联依据；超时不得被映射成 Absent。Submit 结果不确定时必须按稳定 Key Lookup，不能据超时再次生成。

## 领域事件

无。

## 数据模型

无数据库表。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| Request | Key、Kind、Prompt、Images、Model、DurationSeconds、AspectRatio | 稳定生成意图；不含签名 URL |
| Reference | opaque string | 只由实现解释的生成身份 |
| LookupResult | State、Reference | Found / Absent / Uncertain |
| Observation | State、Artifacts、Reason | 生成可观察事实 |
| Download | ArtifactID、LocalPath | 确切产物与本地文件绑定 |

### 约束与索引

无存储索引。Images 顺序不可打乱；下载不能按目录最新文件猜测结果。

## 能力详细说明

Generator 的四个方法均接收 context。Lookup 的 Absent 必须是实现提供的权威否定；Uncertain 表示不允许盲重发。FetchArtifacts 的 Download 必须对应请求中确切 ID。首次生产接入只开放 doubao.i2v.first_frame_prompt@1（恰好一图，VIDEO），该能力校验与资格恢复编排在确认后的完整迁移落地。生产 profile 仅声明这一项能力，其他 fixture 不代表真实生成支持。

## 关联关系

### 依赖的 Domain

无。

### 被依赖的 Domain

无。

### 参与的 Workflow

当前生产调用由 generate.Workflow 提供。已完成 Workflow 判定：真实生成涉及意图提交、外部生成、检查点、上传与交付，必须由后续跨域 Workflow 协调；实际编排见 [generate](../../workflow/0002-generate/generate.md)。

### 关联术语

Generation Request、Reference、Artifact。

## Studio 实现映射

稳定 Key 对应 Worker 专用控制接口的确定任务身份。Lookup 只有在主台账严格校验成功且确切不存在时返回 Absent；损坏/丢失台账、网络错误、未实现路由均为 Uncertain。任务创建与 start/确认意图均经过正式仓储并同步后再触发副作用；Worker 任务保留稳定身份，不允许 UI 删除或重试生成。

Observe 可以驱动实现内部已持久化的单次启动、参数确认和原会话只读观察。参数确认只对原账号 partition 与精确 conversationUrl 匹配的唯一 guest 派发可信输入；不开放外部 CDP 端口。FetchArtifacts 从确切 Artifact ID 下载，通过下载 job ID 回读文件后输出二进制，不向控制客户端公开浏览器内部 URL 或绝对路径。Download 增加已完成文件的 ChecksumSHA256 与 SizeBytes，供 manifest 验证。
