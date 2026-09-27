# 客户端 Capability Worker 经统一网关和 media_workflow 代理访问 Conductor

客户端 `media_worker` 不直接连接 Conductor。它携带 opaque Service Token 请求统一网关；Oathkeeper 或 Edge Auth 调用 `media_auth` 内省凭证并签发短时 Internal Identity JWT；`media_workflow` 的 Worker Gateway API 完成任务授权、持久 Dispatch、输入输出校验，并在内网代理 Conductor poll/update。

Conductor 按 Task Type 和 Task Domain 领取任务，不支持安全的“任意类型”领取。Worker 因此只声明已发布的 `(capability_key, capability_version)`；`media_workflow` 通过 Capability Registry 映射 `conductor_task_def`，并在启动 Workflow 时用 `taskToDomain` 为 Capability Task 设置 Execution Domain。MVP 按提交 User 隔离；Organization 共享 Worker Pool 必须以后通过显式授权引入。客户端不能指定 Domain，队列不能回退到 `NO_DOMAIN`。

每次领取先在 `media_workflow` 持久化 `WorkerDispatch`，再返回不透明 `dispatch_id` 和经过清洗的输入。完成/失败先以 `(dispatch_id, attempt, operation)` 幂等写入 Worker Result Inbox 并返回收据，再由后台适配器重试更新 Conductor。这样 Conductor 的瞬时故障不会迫使客户端重复本地模型或 Provider 副作用。心跳同时维护本地租约与 Conductor response timeout；过期或取消的 Dispatch 不接受晚到结果。

原始 Service Token、Provider 凭证、临时 URL 和 Conductor 标识不得进入 Workflow DSL、Executable IR、Conductor Definition、业务日志或客户端任务契约。编译产物保存 Object Path；领取时签发短时下载/上传地址，Worker 只回传受控 Object Path 及摘要，最终 Generated Asset 由服务端 Finalizer 登记。

选择该方案的原因：

- 统一外部入口、凭证撤销、限流、审计和 trace 传播；
- 在领取前完成 User/Organization/Job/Capability 隔离，避免共享队列先 poll 后鉴权造成任务泄露和饥饿；
- 隐藏 Conductor 内部协议，使客户端契约能独立版本演进；
- 通过 Dispatch/Inbox 把客户端副作用与 Conductor 可用性解耦；
- 保留 ADR 0016 确定的客户端计算与凭证托管边界。

未采用的方案：客户端直连 Conductor 会绕过平台身份和资源授权；共享队列领取后再检查 owner 会占用非本用户任务；客户端提交任意 task type 会暴露 Internal Execution Task；同步更新 Conductor 成功后才响应会把不确定重试转嫁给客户端。

代价是 `media_workflow` 需要维护 Capability poll multiplexer、Dispatch 租约、Result Inbox、Conductor 适配器和额外可观测性。MVP 可按每个 Worker 的每个 Capability 建立一个长轮询，后续在不改变外部 API 的前提下合并为服务端共享 poller。

**状态：accepted**
