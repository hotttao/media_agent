# Personal Capability Worker 运行在客户端

> 2026-09-28：Capability 不再统一限定为客户端执行。本文只保留 `execution_target=personal_worker` 的边界；Managed Worker 与委托身份由 [ADR 0018](0018-talos-api-token-and-delegated-actor.md) 补充。

`execution_target=personal_worker` 的 Execution Capability 运行在用户环境；服务端 `media_workflow` 仍运行 Renderer/Finalizer，并可为 `execution_target=managed_worker` 的 Capability 运行 Managed Worker。客户端以 `media_worker` 工具或常驻服务承载多个 Personal Capability Worker，可访问本地模型、GPU、CLI 与 Provider 凭证，凭证不上传服务端。

该边界支持企业客户把昂贵计算、私有模型和 Provider 凭证留在自己的环境，同时保持服务端 Workflow、Template 和调度模型一致。代价是客户端必须处理注册、心跳、重试、版本兼容与离线恢复；Capability Version 因而成为客户端与服务端的稳定契约，Conductor Task Definition 只是服务端内部映射，服务端不得根据在线 Worker 临时改变已发布 Workflow。

联调允许 `media_worker` 使用显式 Mock Profile，根据 capability task 返回从旧系统选定样本冻结的结果。Mock Profile 只能由启动参数开启，输出必须包含来源标识，不能静默替代生产执行。

**状态：accepted-partially-superseded**
