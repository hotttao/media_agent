# library_object_resolution 变更记录

## 2026-09-23 · 固化短期地址直读流程

- `media_materials` 负责 Organization Membership 和 Object Path 前缀授权。
- Storage Port 先确认对象存在，再生成默认五分钟有效的 S3 Presigned GET URL。
- Browser 直接从 Storage Provider 获取字节，`media_materials` 不代理文件流。
- 明确 Presigned URL 是短期 bearer credential，不得进入业务表或长期缓存。
