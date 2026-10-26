---
name: execution
description: 客户端执行身份与待交付结果事实
dependencies: []
related_workflow:
  - execute
---

## 一句话职责

定义 Task、Capability、Result、Checkpoint 和执行/交付端口，调度与租约的权威状态属于 media_workflow。

## 边界

### 做什么

拥有客户端执行和交付事实，隔离执行器、网关、上传和本地仓储。

### 不做什么

不调度任务，不访问文件/网络，不解释具体生成器引用，不复制 HTTP 或磁盘 DTO。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 注册、领取、心跳、交付 | Worker Gateway 领域端口 | Gateway |
| 声明并执行能力 | 接收 Task，返回动态 candidates/provenance 输出 | Executor |
| 授权上传 | 精确 Task/Attempt/候选位置/本地文件，返回确认 object_path | Uploader |
| 待交付结果恢复 | 保存、列出、完成 | ResultRepository |
| 生成事实恢复 | 按 Task/Attempt 保存意图摘要、引用、产物 manifest 与已确认 Object Path | CheckpointRepository |
| 本地执行准入 | 专用实例容量与就绪，恢复准入不要求剩余生成额度 | Admission / RecoveryAdmission |
| 输入与产物文件 | 限制输入媒体；验证 manifest 的大小与摘要 | InputDownloader / ArtifactVerifier |
| 恢复生命周期 | 注册并重放结果后恢复未关闭的执行 | ExecutionRecovery |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Result | Task 身份、原 key、输出或失败 | 单文件原子写入 | 同 key 同事实幂等；不同事实拒绝覆盖；确认前保留 |
| Checkpoint | 意图身份、请求摘要、不透明引用、产物事实 | 后续仓储事务 | 不承载调度与资格 |

Task 是网关执行信封的本地值，Capability 是能力声明。Result 与 Checkpoint 独立持久化，避免把未完成生成冒充待交付结果。

## 领域事件

无。

## 数据模型

无数据库表。领域结构无序列化标签。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| Task | DispatchID / Attempt / Capability / Input / LeaseExpiresAt / TraceParent | 执行信封；Input 为动态 map，无重复 input_json |
| Result | Task / Key / Output / Reason / FailureKind | 已确定的交付事实 |
| Checkpoint | Task / Intent / GenerationKey / RequestDigest / Reference / Artifacts / Closed | 生成恢复事实；Reference 不解释为 studio ID |

### 约束与索引

现有日志命名空间和文件键的 SHA-256 规则保持不变，disk DTO 保留旧 snake_case、空 input_json 字段与权限。只存交付所需身份，不保存提示词输入或 Token。

## 能力详细说明

Gateway Complete/FailWithKind 仅在 submitted 回执时成功；客户端将 409/410 转成 ErrResultRejected，workflow 不感知 HTTPError。Executor 可返回实现 Permanent() 的错误，policy 负责分类。Uploader 保持候选索引、摘要授权、PUT 与重新申请授权确认的幂等行为。ResultRepository Save/Pending/Finish 的副作用由 results 实现；失败必须返回到入口，不能在业务协程内退出进程。

## 关联关系

### 依赖的 Domain

无，execution 与 doubao 不互相 import。

### 被依赖的 Domain

无。

### 参与的 Workflow

[execute](../../workflow/0001-execute/execute.md) 协调全部执行与交付副作用。

### 关联术语

Task、Capability、Result、Checkpoint。

## 生成恢复规则

`ErrExecutionPending` 保留同一 Attempt 和本地容量，不生成失败回执；`ErrExecutionAbandoned` 表达网关已拒绝资格，此时关闭本地检查点并停止副作用。Checkpoint 的 Intent、Key、摘要不可改写，引用只允许首次绑定，manifest 已确认的产物 ID/文件路径/大小/摘要不可漂移，Object Path 只允许首次确认。Closed 不可重新打开。

生产重启先交付结果日志，再通过 ExecutionRecovery 列出检查点身份，串行恢复。检查点不含签名输入 URL、Token 或 HTTP input_json；Result 继续使用原磁盘格式，互不混用。

[generate](../../workflow/0002-generate/generate.md) 负责跨 execution/doubao 的生成、下载和上传；domain 包仍不相互 import。
