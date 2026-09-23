# library_object_upload 变更记录

## 2026-09-21 · 固化企业资源对象上传边界

- 用户确认浏览器统一调用 `media_materials`，由该服务校验用户和 Organization Membership、生成 Object Path，再以服务身份调用 `media_storage`。
- `media_storage` 不接收最终用户身份，不拥有 Organization、Scope、owner 或 visibility 权限模型。
- 上传与 Material/Virtual IP/Product CRUD 保持两个事务；上传成功但未被引用的对象进入待回收集合。
- 使用 Idempotency Key 生成稳定路径，使超时和响应丢失后可安全重试；同路径不同内容返回冲突。
- 明确该流程是跨服务应用 Workflow，不是 Conductor 或 `media_workflow` 的节点执行。

## 2026-09-23 · 标准化为 S3 Storage Port

- 删除自研 media_storage HTTP Put、LocalFile Driver 与 Stored Object 元数据库在上传流程中的职责。
- `media_materials` 通过 S3 Storage Port 直接连接 Storage Provider；本地默认 SeaweedFS，Provider 可配置替换为 MinIO、Aliyun OSS 或 AWS S3。
- 用 `If-None-Match=*` 条件写和 checksum metadata 保持“同路径同内容幂等、不同内容冲突”的既有语义。
- Bucket、Endpoint、凭证和 Presigned URL 均为基础设施配置或临时值，业务表继续只保存 Object Path。
