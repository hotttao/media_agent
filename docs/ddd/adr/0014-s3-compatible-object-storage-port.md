# 以 S3 兼容协议作为对象存储端口

对象存储统一采用 S3 兼容协议，不继续维护自研 LocalFile Driver、对象元数据库、内部对象 HTTP 协议或 URL 签名算法。本地部署使用 SeaweedFS，业务服务通过同一 Storage Port 连接；生产环境只需替换 Endpoint、Region、Bucket、凭证和寻址配置即可接入 MinIO、Aliyun OSS、AWS S3 等实现。业务服务仍拥有最终用户、Organization 与资源授权，存储提供方只执行受 S3 凭证和 Bucket/Prefix Policy 限制的对象操作。

采用该方案是因为自研实现必须长期承担并发写入、条件请求、Range、校验、签名兼容、安全修复和云厂商差异，而这些能力已有成熟 S3 实现。业务库只持久化稳定 Object Path，不持久化 Bucket、Endpoint 或 Presigned URL；因此替换提供方与迁移对象不会改变领域模型。SeaweedFS 是本地默认实现而非领域依赖。

**状态：accepted**

**衍生影响**：上传幂等冲突使用 S3 条件写与对象 checksum metadata 判定；读取由业务服务授权后签发短期 Presigned URL，浏览器直接访问 Storage Provider。对象回收由业务服务或独立生命周期任务调用标准 S3 Delete/Head，不再依赖自研 `stored_objects` 表。
