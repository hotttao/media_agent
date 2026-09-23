# stored_object domain 变更记录

## 2026-09-21 · 明确 Caller Service 边界

- media_storage 只接受受信 Caller Service，不接收浏览器最终用户身份。
- `media-materials` 只能访问 `materials` Namespace；Organization Membership 和资源权限由 media_materials 在调用前完成。
- 当前内部 HTTP 使用独立共享凭证识别服务，领域边界允许后续替换为 mTLS。
- PutObject 以 Object Path 为幂等键：同内容重试成功，不同内容冲突且禁止覆盖。

## 2026-09-23 · 以 S3 兼容 Provider 替代自研存储

- Stored Object 改由 S3 兼容 Provider 持久化，不再维护 `stored_objects` 表、LocalFile Driver 或内部对象 HTTP API。
- 本地默认使用 SeaweedFS；MinIO、Aliyun OSS、AWS S3 通过相同 Storage Port 和配置接入。
- 保留 Object Path 作为业务稳定引用；Bucket、Endpoint、ETag 和 Presigned URL 不进入业务表。
- 上传幂等使用条件 Put 与 `checksum_sha256` metadata，读取使用短期 Presigned URL，Browser 直接访问 Provider。
