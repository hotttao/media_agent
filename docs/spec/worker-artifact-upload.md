# Worker产物对象上传

依据[DDD流程](../ddd/workflow/0001-worker_artifact_upload/worker_artifact_upload.md)。只补当前执行所需上传/输出校验，不做UI、细粒度ACL、真实Provider接入或历史迁移。

## 协议

Worker新JSON端口`POST /api/worker/v1/tasks/{dispatch_id}/uploads`，字段worker_instance_id、attempt、candidate_index、content_type、size_bytes、checksum_sha256、content_md5。复用现有Worker身份和原生Task准入；企业/Scope来自Job，位置0–7；输出类型来自冻结Capability。服务端把Task/attempt/位置哈希为稳定不透明上传键。

Materials新增内部`POST /api/internal/v1/library-objects:prepareUpload`，使用既有内部服务凭证。字段organization_id、scope、upload_key和上述文件元数据；生成`materials/{organization}/{scope}/worker_generated/{task-key}/{manifest-hash}`，无客户端任意路径或扩展名。manifest-hash包含SHA256、MD5、类型、长度，改换内容不能覆盖旧引用；同一物理路径异内容仍冲突。响应object_path、upload_url、headers、expires_at、already_uploaded。

S3 Storage Port增加PresignPut：HEAD已存在对象时核对类型/大小/摘要并返回already_uploaded，否则签发If-None-Match=*和校验和绑定的短期PUT。必须验证现用SeaweedFS实际校验摘要、拒绝覆盖；不能以HTTP假服务替代真实协议门禁。相同内容重放返回原对象，不同内容冲突。Provider仍使用当前S3适配配置，不引入新存储服务。

media_worker增加本地文件上传helper，嗅探现有PNG/JPEG/WebP/GIF/MP4/WebM、限制大小并计算SHA256/MD5；只有网关请求携Worker Token，数据PUT禁止Bearer及自动重定向。上传失败不记录成功；未知或412重新申请并确认原对象。Provider mock配置允许result_files，与result_urls互斥，返回Object Path；旧URL模式保留。

Complete新能力的媒体结果在代理Conductor前检查企业/Scope、类型、非空/上限；拒绝纯文本冒充图片/视频和外部URL。Get/GetStepRun候选Object Path以短期URL在响应副本中展示，不持久化签名、不在查询登记素材。Managed共享受信内部准备流程，不获得User API Token。

## 验收

单测/race：绑定身份、native失效、类型/大小、稳定键、重复文件/冲突、Token不转发、丢响应/412恢复、URL模式兼容、无查询登记。真实SeaweedFS：直接PUT/摘要错误拒绝/条件写/重放HEAD/直接GET字节。公开网关+独立Worker：同一完整四节点mock从本地已知文件上传，执行失败继续、多候选审批/重新生成与四产物读取；只运行一个样本，不迁移全量历史。实现/技术门禁/部署验收分别记录。
