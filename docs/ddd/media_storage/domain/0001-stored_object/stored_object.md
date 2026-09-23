---
name: stored_object
description: S3 兼容对象的稳定键、内容元数据与物理生命周期
dependencies: []
related_workflow:
  - library_object_upload
  - library_object_resolution
  - object_retention_gc
---

## 一句话职责

Stored Object 以 Object Path 为稳定键保存二进制内容和对象元数据，并通过标准 S3 能力提供条件写、读取、查询、删除与短期地址。

## 边界

### 做什么

- 通过 S3 Storage Port 提供 PutIfAbsent、Head、Delete 与 PresignGet。
- 由 Storage Provider 原子保存对象字节与 content type、checksum 等 metadata。
- 用 S3 凭证和 Bucket/Prefix Policy 限制 Caller Service 的物理访问范围。
- 本地默认由 SeaweedFS 实现；MinIO、Aliyun OSS、AWS S3 通过相同端口接入。

### 不做什么

- 不决定 Organization Membership、业务 owner、ACL、分享、引用关系或何时删除。
- 不生成业务 Object Path，不把 Bucket、Endpoint 或 Presigned URL写入业务事实。
- 不维护自研对象元数据库、自研 URL 签名算法或文件传输代理。
- 不承诺供应商专有能力；业务流程只能依赖规格明确列出的 S3 兼容子集。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 条件写入对象 | 不覆盖已有不同内容，支持相同内容幂等重试 | `ObjectStorage.PutIfAbsent` |
| 查询对象 | 返回 size、content type、checksum、ETag | `ObjectStorage.Stat` |
| 删除对象 | 幂等删除指定 Object Path | `ObjectStorage.Delete` |
| 解析读取地址 | 确认对象存在后签发短期读取地址 | `ObjectStorage.PresignGet` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Stored Object | Object Path、字节、content type、checksum metadata、ETag | Storage Provider 的单对象条件写 | 同一 Object Path 已存在不同 checksum 时禁止覆盖；对象 metadata 与字节在一次 PutObject 中提交 |

`Object Path` 是值对象；Bucket 和 Endpoint 是部署配置，不属于聚合身份。Provider 内部如何分片、复制、纠删码或落盘不进入领域模型。

## 领域事件

无。Storage Provider 的对象通知不作为首期业务事实来源；业务服务不能依赖它推导资源创建成功。

## 数据模型

本上下文不维护应用数据库表。对象及 metadata 由 Storage Provider 持久化：

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| bucket | provider configuration | 物理对象容器，不进入业务表 |
| object_key | string | 等于业务 Object Path |
| content_type | MIME string | 对象媒体类型 |
| checksum_sha256 | S3 user metadata | 幂等与冲突判定使用的内容校验值 |
| content_length | int64 | 对象大小 |
| etag | provider value | Provider 返回的版本/内容标识，不作为跨 Provider checksum |

业务聚合只保存 `object_path`。不得把 ETag 当作 SHA-256：不同 Provider 和 multipart 行为可能产生不同 ETag 语义。

## 能力详细说明

### PutIfAbsent

1. Caller Service 在调用前生成合法 Object Path 并计算 SHA-256。
2. Storage Port 用配置 Bucket 和 Object Path 执行 HeadObject；已存在时比较 `checksum_sha256`。
3. checksum 相同返回幂等成功；不同返回 conflict。
4. 不存在时使用 `If-None-Match: *` 条件 PutObject，并写入 content type 与 checksum metadata。
5. 条件失败表示并发写入获胜；再次 HeadObject，比对 checksum 后收敛为成功或 conflict。
6. 超时导致结果不明确时不得盲目换 Key，调用方使用相同 Object Path 查询并重试。

### Stat

HeadObject 返回对象 metadata。NotFound 与 Provider Unavailable 必须区分；日志不得输出 Access Key、Secret Key、完整 Presigned URL。

### PresignGet

1. 业务服务已经完成最终用户和资源范围授权。
2. Storage Port 先 HeadObject，存在后才签发短期 URL。
3. URL 有效期默认五分钟且不得超过配置上限。
4. Browser 直接访问 Storage Provider；业务服务不代理对象字节。

### Delete

DeleteObject 是物理副作用，只有拥有业务生命周期的服务或 Retention Workflow 可以触发。NotFound 视为幂等成功；结果不明确时先 HeadObject 再决定是否重试。

## 关联关系

### 依赖的 Domain

- 无。

### 被依赖的 Domain

- `media_materials` 的 Material、Virtual IP、Product 与 Generated Asset 只保存 Object Path。
- `media_gen` 的生成结果和转写结果只保存 Object Path。
- `media_control` 的导出对象只保存 Object Path。

### 参与的 Workflow

- [library_object_upload](../../../workflow/0008-library-object-upload/library_object_upload.md) - 接受经业务授权的对象条件写。
- [library_object_resolution](../../../workflow/0009-library-object-resolution/library_object_resolution.md) - 为经业务授权的读取签发短期地址。
- [object_retention_gc](../../workflow/0001-object_retention_gc/object_retention_gc.md) - 执行到期物理删除。

### 关联术语

- `Object Path`
- `Stored Object`
- `S3 Storage Port`
- `Storage Provider`
- `Resolved URL`
