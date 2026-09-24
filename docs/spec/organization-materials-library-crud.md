# Organization Materials Library CRUD Spec

## 1. Scope

第一阶段只实现 Organization 内共享的 Material、Virtual IP、Product CRUD。显式 Binding、Reachability/Workflow、生成、发布和 IP 专属素材不在范围内。

`organization_id` 是资源隔离与共享边界；`scope` 是 Organization 内不可变业务品类。所有查询必须同时限定 Organization，跨 Organization 返回不可达。

## 2. Authentication and authorization

- `user_id` 从认证中间件取得，不接受请求字段覆盖。
- 请求显式携带 `organization_id`，但服务必须通过 media_auth 的内部授权接口确认当前 user 是 active Organization Member；校验失败时 fail closed。
- Material、Virtual IP：owner 可写；读取按 owner/visibility/team grant 判断，`public` 只在 Organization 内公开。
- Product：无资源级 ACL；active Organization Member 在同 Scope 内共享 CRUD。
- Material 列表缺省 `only_mine=true`；显式 false 才返回同 Organization/Scope 内其他可见记录。

## 3. Storage contract

业务库禁止保存 provider、bucket、域名、ETag 或签名 URL，只保存由业务服务生成的稳定 `object_path`。`media_materials` 定义 S3 Storage Port，提供 `PutIfAbsent/Stat/Delete/PresignGet`；本地 `media-storage` 部署使用 SeaweedFS，MinIO、Aliyun OSS 与 AWS S3 通过同一端口和配置替换。

上传时 Browser 只调用 media_materials 的 `POST /api/v1/library-objects:upload`。media_materials 从认证上下文取得 caller，校验 active Organization Membership、Scope、用途、媒体类型与大小，生成 Object Path 和 SHA-256 后，以受限 S3 凭证调用 Storage Provider。Provider 的 Bucket/Prefix Policy 必须把该凭证限制到 `materials/` Namespace；用户会话不得作为 S3 凭证。

上传与资源 CRUD 是两个事务。上传成功返回 Object Path、size、content type、checksum，不自动创建或更新业务聚合。后续 CRUD 失败时不立即删除对象，未引用对象由 Retention/GC 后续回收。

### 3.1 Public upload API

`POST /api/v1/library-objects:upload?organization_id=&scope=&purpose=`，`multipart/form-data` 的文件字段固定为 `file`，请求头必须包含 `Idempotency-Key`。

- `purpose` 闭集合：`material`、`virtual_ip_avatar`、`virtual_ip_full_body`、`virtual_ip_three_view`、`virtual_ip_nine_view`、`product_image`。
- 首期只接受 `image/jpeg`、`image/png`、`image/webp`、`image/gif`，最大 16 MiB；空文件拒绝。该上限必须容纳历史高分辨率身份图的无损迁移。
- Hertz `server.max_request_body_bytes` 至少为 18 MiB，为 16 MiB 文件保留 multipart envelope 空间；业务 workflow 仍以文件本身的 16 MiB 为硬边界。
- Object Path 格式：`materials/{organization_id}/{scope}/{purpose}/{sha256(caller_id + idempotency_key)[0:32]}.{ext}`。Organization、Scope、purpose 先按安全段规则校验，文件原名不进入目录。
- 同一 caller + Idempotency Key + 上传语义产生稳定路径；同内容重试成功，不同内容返回 409。
- 非 Organization Member 返回 404；非法请求 400；超限 413；Storage 不可用或结果不明确 503。

### 3.2 S3 Storage Port

Storage Port 位于 `media_materials/biz/shared/objectstorage`，不得依赖 Hertz DTO。实现使用 AWS SDK for Go v2 的 S3 client 与 presign client：

- `PutIfAbsent`：先 HeadObject；已有 `checksum_sha256` 相同则返回幂等成功，不同则 conflict。不存在时执行 `If-None-Match: *` PutObject 并写 content type 与 checksum metadata；并发条件失败后再次 HeadObject 收敛。
- `Stat`：区分 not found 与 unavailable，返回 size、content type、checksum、ETag；ETag 不作为 SHA-256。
- `PresignGet`：先 Stat 确认对象存在，再生成默认五分钟有效的 URL 与 `expires_at`。
- `Delete`：NotFound 视为幂等成功；只允许由业务生命周期或 Retention Workflow 调用。

Storage 配置必须包含 provider、endpoint、region、bucket、access key、secret key、path-style 与 presign TTL。`provider` 首期统一为 `s3`，其他字段允许按部署替换。配置缺失必须启动失败；Secret 不写入日志。

部署配置只改变 Adapter 参数，不改变业务代码：

| Provider | endpoint / public_endpoint | region | force_path_style | 说明 |
|:---|:---|:---|:---|:---|
| SeaweedFS（本地默认） | `http://media-storage:8333` / Browser 可达的 `http://<LAN-IP>:8333` | `us-east-1` | `true` | Compose 服务名固定为 `media-storage` |
| MinIO | MinIO S3 API 的内外网 Endpoint | 部署 Region | `true` | Bucket 与受限 Access Key 预先创建 |
| Aliyun OSS S3 compatibility | 对应 Region 的 S3 兼容 Endpoint | 对应 OSS Region | `false` | 使用支持 S3 协议的 Endpoint 与 RAM 最小权限凭证 |
| AWS S3 | endpoint 与 public endpoint 留空，使用 SDK 默认地址 | AWS Region | `false` | IAM Policy 只授权目标 Bucket 的 `materials/*` |

`endpoint` 是业务服务访问地址；`public_endpoint` 只决定签名 URL 中 Browser 访问的 Host。二者指向同一对象命名空间且签名端必须与实际请求 Host 一致，不能把已签名请求任意改写到业务网关。

### 3.3 Public resolution API

`GET /api/v1/library-objects:resolve?organization_id=&object_path=` 由 media_materials 认证。caller 不是 active Organization Member 返回 404；Object Path 不以 `materials/{organization_id}/` 开头返回 403；对象不存在返回 404；Provider 不可用返回 503。

成功返回 `{url, expires_at}`。Browser 直接请求 Presigned URL，media_materials 和业务 API 网关不代理对象字节。本地开发直接访问 SeaweedFS 的公开 S3 端口；生产云 Provider 返回 OSS/S3 的外部 Endpoint 或对象存储专用域名。业务表和日志不得持久化完整 Presigned URL。

## 4. Database

以追加 Migration 修改现有 schema，不重写 `000001_init`：

- materials：增加 organization_id、description、prompt、object_path、metadata；旧 url 仅为迁移兼容列，不映射成 Object Path，也不由新接口读写；生产迁移必须另行把旧对象导入 media_storage；唯一键 `(organization_id, scope, asset_kind, name)`（未软删）。
- virtual_ips：增加 organization_id、四个 `*_object_path`、persona_profile；唯一键 `(organization_id, scope, name)`。
- products：增加 organization_id、display_actions jsonb、tags jsonb、external_commerce jsonb、metadata jsonb；唯一键 `(organization_id, scope, name)`。
- product_images：Product 聚合内子表；`(product_id, sort_order)` 唯一，同 Product 至多一张主图。
- organization_id、scope 创建后不可变；软删释放业务唯一键。

开发数据库已有旧数据时，Migration 使用明确的 legacy organization 占位值完成 NOT NULL 迁移；生产迁移前必须用真实 Organization 映射替换占位记录。

## 5. HTTP contract

三个资源的 Create/List 请求必须包含 `organization_id`；Get/Update/Delete 从记录反查 Organization 后授权。

- Material：返回 description、prompt、object_path、content、metadata；List 支持 scope、asset_kind、visibility、only_mine、分页。
- Virtual IP：返回四类身份图片 object path 与结构化 persona_profile JSON；List 支持 scope、name、status、visibility、分页。
- Product：Create/Update 可原子提交 images；返回 tags、display_actions、external_commerce、metadata 和有序 images；List 支持 scope、name、target_audience、tag、status、分页。

唯一键冲突统一映射 HTTP 409；不存在或越权统一映射 404；非法字段 HTTP 400；media_auth 不可用 HTTP 503。

## 6. Harness mapping

- 聚合和值对象：`media_materials/biz/domain/{material,virtual_ip,product}`。
- 单聚合用例：`media_materials/biz/service/<domain>`。
- PostgreSQL 实现：`media_materials/biz/dal/repo/<domain>`。
- DTO 适配：`idl/media_materials` 与 `biz/handler`。
- Organization Membership client：`media_materials/biz/shared/auth`；Handler 只消费认证上下文，不信任请求中的 owner。
- 上传编排：`media_materials/biz/workflow/library_object_upload`；Storage Port 与 S3 adapter 放 `media_materials/biz/shared/objectstorage`，不得 import Hertz DTO。
- 地址解析：`media_materials/biz/handler/library_object` 完成 Organization/path 授权后调用 Storage Port；Browser 直连 Provider。
- 本地 Storage Provider：`docker-compose/docker-compose.storage.yml`，服务名 `media-storage`，持久化目录挂载到 SeaweedFS `/data`；不启用额外可观测性服务。

## 7. Tests

- Domain/Service 单元测试覆盖名称、载荷、object path、owner、only_mine、Organization 隔离和 Product Image 不变量。
- Repository 测试覆盖部分唯一索引、软删后重建、Product + Images 原子更新。
- HTTP e2e 放 `media-cli/e2e/`，通过统一网关登录后完成三类资源 CRUD；服务和基础设施由 docker-compose 启动，测试自身不管理进程。
- S3 adapter 使用 `httptest.Server` 或 S3-compatible test double 覆盖签名配置、Head/Put 条件竞争、checksum 幂等、冲突、not found 与 presign TTL。
- upload workflow 使用 fake Storage Port 覆盖路径确定性、用途/媒体类型/大小校验、冲突和不可用映射。
- HTTP E2E 经统一网关完成登录、上传、resolve，并使用返回的 URL 直接读取图片；断言 URL 请求不经过 media_materials 字节代理。

## 8. Migration capability coverage

| 原能力 | 目标限界上下文 | Domain / Workflow | 状态 | 说明 |
|:---|:---|:---|:---|:---|
| Material CRUD | media_materials | asset | covered | 第一阶段已实现 |
| Virtual IP CRUD 与身份图片引用 | media_materials | virtual_ip + library_object_upload | covered | 第一阶段已实现 |
| Product CRUD 与有序图片 | media_materials | product + library_object_upload | covered | 第一阶段已实现 |
| 浏览器文件上传 | media_materials / media_storage | library_object_upload + stored_object | covered | media_materials 校验并通过 S3 Port 写入 |
| 企业对象短期读取 | media_materials / media_storage | library_object_resolution + stored_object | covered | Browser 使用 Presigned URL 直连 Provider |
| Workflow 生成产物 | media_workflow / media_materials | workflow_execution + generated_asset | external-owner | 本期不迁移，由 media_workflow 后续执行链拥有 |
| Material Binding / Reachability | media_materials | asset + reachability | deprecated-with-reason | 第一阶段明确不开放写入口，保留后续领域设计 |
| Pose / Movement Compatibility | media_materials | asset | deprecated-with-reason | 第一阶段明确排除 |
| 旧 IpMaterial | — | — | deprecated-with-reason | 用户明确暂不考虑 IP 专属素材 |
| 发布计划与统计 | — | — | deprecated-with-reason | 不属于 Materials 第一阶段 |

本期能力矩阵没有 `missing`；标记 `deprecated-with-reason` 的条目是明确排除的迁移能力，不得出现于第一阶段公开路由。
