# stored_object domain 变更记录

## 2026-09-21 · 明确 Caller Service 边界

- media_storage 只接受受信 Caller Service，不接收浏览器最终用户身份。
- `media-materials` 只能访问 `materials` Namespace；Organization Membership 和资源权限由 media_materials 在调用前完成。
- 当前内部 HTTP 使用独立共享凭证识别服务，领域边界允许后续替换为 mTLS。
- PutObject 以 Object Path 为幂等键：同内容重试成功，不同内容冲突且禁止覆盖。
