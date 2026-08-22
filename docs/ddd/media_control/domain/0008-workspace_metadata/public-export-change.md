# Public Export 变更记录

## 2026-08-22：公共导出元数据生命周期

- 新增 storage-relative object path 的 Public Export 记录。
- 导出状态由控制面维护；只有 ready 状态具备对外发布语义，二进制仍归 media_storage。
