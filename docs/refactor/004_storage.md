# 004 存储规范

## 1. 服务定位

`media_storage` 是薄存储服务，只负责：

- 上传、读取、查看元数据和删除对象。
- 校验调用来源是否合法。
- 屏蔽本地文件系统、阿里云 OSS 等存储实现差异。
- 将对象路径解析为可访问 URL。

它不判断对象属于谁、能否分享或何时因业务原因删除。这些规则由上传对象的业务服务负责。

## 2. 对象路径

- 调用方生成域名无关的相对路径，并负责命名规则。
- 业务数据库只保存路径，不保存完整 URL。
- 路径必须包含稳定的服务命名空间，禁止 `..` 和绝对路径。

示例：

```text
materials/ws_01/2026/08/mat_01/original.png
workflow/ws_01/run_01/output/video.mp4
```

```text
media_materials.materials
id          mat_01
workspace   ws_01
object_path materials/ws_01/2026/08/mat_01/original.png
mime_type   image/png
status      active
```

业务服务需要展示资源时调用 `ResolveURL(object_path)`；域名或存储后端变化不需要改业务记录。

## 3. 接口语义

| 操作 | 说明 |
|---|---|
| `Put(path, stream, metadata)` | 由 Storage 直接接收上传，不引入 `upload_url` 概念 |
| `Open(path)` | 读取对象流 |
| `Stat(path)` | 获取大小、类型和校验值 |
| `Delete(path)` | 执行业务服务已决定的删除 |
| `ResolveURL(path, ttl)` | 返回内部地址或短时访问地址 |

## 4. 驱动与安全

- 首期驱动：`LocalFile`、`AliyunOSS`；驱动由配置选择。
- Storage 根据 mTLS 服务身份校验允许访问的命名空间。
- 生产环境不接受客户端伪造的服务名 Header。
- 业务分享链接由资源所属服务签发受限随机 Token，Storage 不维护业务分享 ACL。
- 删除采用可配置回收期；派生资源暂不纳入设计。
