# media_storage 对象存储上下文

以标准 S3 协议保存和读取二进制对象，屏蔽 SeaweedFS、MinIO、Aliyun OSS 与 AWS S3 等实现差异。它只按存储凭证和策略执行对象操作，不拥有对象的业务权限。

## 业务术语词典

**Object Path（对象路径）**:
由业务服务生成、域名无关且带服务命名空间的稳定相对路径。
_禁用替代词_：URL、File Name、Object ID

**Stored Object（存储对象）**:
由 Object Path 唯一标识的二进制内容及其大小、媒体类型和校验值元数据。
_禁用替代词_：Material、Asset、Business Resource

**Storage Namespace（存储命名空间）**:
Object Path 的首段，用于约束持有存储凭证的业务服务可访问的键前缀。
_禁用替代词_：Bucket、Workspace

**Caller Service（调用服务）**:
持有受限 S3 凭证并调用对象能力的上游业务服务；每个 Caller Service 只可访问被授予的 Bucket 与 Storage Namespace。最终用户身份不能替代 Caller Service 身份。
_禁用替代词_：End User、Browser Client、Anonymous Caller

**S3 Storage Port（S3 存储端口）**:
业务服务使用的统一对象操作契约，其实现由 Endpoint、Region、Bucket、凭证和寻址方式配置，不暴露供应商专有模型。
_禁用替代词_：LocalFile Driver、Provider SDK

**Storage Provider（存储提供方）**:
实现 S3 兼容对象协议的部署实例；本地默认是 SeaweedFS，也可以是 MinIO、Aliyun OSS、AWS S3 或其他兼容服务。
_禁用替代词_：Storage Driver、Business Service

**Resolved URL（解析地址）**:
业务服务完成资源授权后，由 Object Path 生成的短期 Presigned URL；浏览器凭该地址直接读取 Storage Provider，地址不得作为业务事实持久化。
_禁用替代词_：Object Path、Public URL

**Retention Period（回收期）**:
对象被标记删除后到物理清理前的可配置保留时间。
_禁用替代词_：Business Lifecycle、TTL
