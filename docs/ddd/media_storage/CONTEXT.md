# media_storage 对象存储上下文

以统一路径接口保存和读取二进制对象，屏蔽本地文件系统与云对象存储差异。它只验证调用服务，不拥有对象的业务权限。

## 业务术语词典

**Object Path（对象路径）**:
由业务服务生成、域名无关且带服务命名空间的稳定相对路径。
_禁用替代词_：URL、File Name、Object ID

**Stored Object（存储对象）**:
由 Object Path 唯一标识的二进制内容及其大小、媒体类型和校验值元数据。
_禁用替代词_：Material、Asset、Business Resource

**Storage Namespace（存储命名空间）**:
Object Path 的首段，用于约束已认证服务可访问的路径范围。
_禁用替代词_：Bucket、Workspace

**Storage Driver（存储驱动）**:
实现统一对象操作的物理后端适配器，首期包括 LocalFile 和 AliyunOSS。
_禁用替代词_：Storage Service、Bucket

**Resolved URL（解析地址）**:
由 Object Path 临时解析出的内部地址或有期限访问地址，不作为业务事实持久化。
_禁用替代词_：Object Path、Public URL

**Retention Period（回收期）**:
对象被标记删除后到物理清理前的可配置保留时间。
_禁用替代词_：Business Lifecycle、TTL
