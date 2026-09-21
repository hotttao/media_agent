# library_object_upload 变更记录

## 2026-09-21 · 固化企业资源对象上传边界

- 用户确认浏览器统一调用 `media_materials`，由该服务校验用户和 Organization Membership、生成 Object Path，再以服务身份调用 `media_storage`。
- `media_storage` 不接收最终用户身份，不拥有 Organization、Scope、owner 或 visibility 权限模型。
- 上传与 Material/Virtual IP/Product CRUD 保持两个事务；上传成功但未被引用的对象进入待回收集合。
- 使用 Idempotency Key 生成稳定路径，使超时和响应丢失后可安全重试；同路径不同内容返回冲突。
- 明确该流程是跨服务应用 Workflow，不是 Conductor 或 `media_workflow` 的节点执行。
