# Media Workflow Execution 变更记录

## 2026-09-28 · Personal 与 Managed Worker

- Capability Version 冻结 execution_target，跨服务执行流程按 Personal User Domain 或 Managed Pool Domain 分支。
- Personal Worker 使用 Talos API Token 委托身份；Managed Worker 位于 media_workflow 服务边界内，不请求 Talos。
- 两条路径在 WorkerDispatch/Result Inbox 汇合，统一保持结果幂等和 Conductor 故障隔离。

## 2026-09-27 · 客户端能力执行安全边界

- Personal `media_worker` 经 Gateway/Edge Auth 使用 opaque API Token 获取短时委托 Internal Identity JWT，不再连接 Conductor。
- `media_workflow` 负责 Capability 授权、Task Domain 隔离、Dispatch 租约、结果 Inbox 和 Conductor 代理。
- Worker 输出先进入受控对象存储，服务端 Finalizer 再调用 `media_materials` 登记 Generated Asset；客户端回写结果与服务间资源登记被明确拆分。
