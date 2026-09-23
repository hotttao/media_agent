# object_retention_gc 变更记录

## 2026-09-23 · 改为业务引用复核后的 S3 删除

- 删除对自研 Storage DB、River 状态和 Storage Driver 的依赖。
- Retention Worker 在物理删除前回到资源权威服务复核引用与 deletion version，避免晚到任务删除已恢复对象。
- 物理删除通过标准 S3 DeleteObject 执行；NotFound 视为幂等成功，超时先 HeadObject 再重试。
- 禁止仅按对象创建时间配置 Bucket Lifecycle 删除业务对象。
