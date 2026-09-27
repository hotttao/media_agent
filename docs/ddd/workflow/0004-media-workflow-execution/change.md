# Media Workflow Execution 变更记录

## 2026-09-27 · 客户端能力执行安全边界

- `media_worker` 统一经 Gateway/Edge Auth 使用 opaque Service Token 获取短时 Internal Identity JWT，不再连接 Conductor。
- `media_workflow` 负责 Capability 授权、Task Domain 隔离、Dispatch 租约、结果 Inbox 和 Conductor 代理。
- Worker 输出先进入受控对象存储，服务端 Finalizer 再调用 `media_materials` 登记 Generated Asset；客户端回写结果与服务间资源登记被明确拆分。
