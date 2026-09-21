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

业务库禁止保存 provider、bucket、域名或签名 URL，只保存由业务服务生成的稳定 `object_path`。media_storage 的 Stored Object Driver 提供 `Put/Open/Stat/Delete`，首个实现为受配置根目录约束的 LocalFile；MinIO/OSS 后端后续替换，不影响 Domain 端口。

浏览器只调用 media_materials 的 `POST /api/v1/library-objects:upload`，不得直连 media_storage。media_materials 从认证上下文取得 caller，校验 active Organization Membership、Scope、用途、媒体类型与大小，生成 Object Path 后，以 `media-materials` 服务身份调用 media_storage 内部 PutObject。media_storage 只认证 Caller Service 并将其限制到 `materials` Namespace，不读取用户会话，也不做 Organization/owner/visibility 判断。

上传与资源 CRUD 是两个事务。上传成功返回 Object Path、size、content type、checksum，不自动创建或更新业务聚合。后续 CRUD 失败时不立即删除对象，未引用对象由 Retention/GC 后续回收。

### 3.1 Public upload API

`POST /api/v1/library-objects:upload?organization_id=&scope=&purpose=`，`multipart/form-data` 的文件字段固定为 `file`，请求头必须包含 `Idempotency-Key`。

- `purpose` 闭集合：`material`、`virtual_ip_avatar`、`virtual_ip_full_body`、`virtual_ip_three_view`、`virtual_ip_nine_view`、`product_image`。
- 首期只接受 `image/jpeg`、`image/png`、`image/webp`、`image/gif`，最大 4 MiB；空文件拒绝。
- Object Path 格式：`materials/{organization_id}/{scope}/{purpose}/{sha256(caller_id + idempotency_key)[0:32]}.{ext}`。Organization、Scope、purpose 先按安全段规则校验，文件原名不进入目录。
- 同一 caller + Idempotency Key + 上传语义产生稳定路径；同内容重试成功，不同内容返回 409。
- 非 Organization Member 返回 404；非法请求 400；超限 413；Storage 不可用或结果不明确 503。

### 3.2 Internal storage API

`PUT /api/internal/v1/objects?object_path=`，请求体为原始字节，`Content-Type` 是对象媒体类型；调用方携带 `X-Media-Storage-Internal-Token`。配置未设置、凭证不匹配、Caller Service 不允许 `materials` Namespace 时均 fail closed。

首期 Caller Service 与 Namespace 是静态映射：该凭证仅代表 `media-materials`，只能写 `materials/`。内部接口返回 `object_path`、`size_bytes`、`content_type`、`checksum`；非法路径 400、未认证 401、越权 Namespace 403、内容冲突 409、后端失败 500。

读取接口不在本次上传里程碑开放。通用 `object_path` 读取会绕过 Material/Virtual IP 的资源可见性；后续必须按已绑定业务资源完成授权后代理读取或签发短期 URL，不能仅凭 Organization Membership 暴露任意对象。

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
- 上传编排：`media_materials/biz/workflow/library_object_upload`；Storage client 放 `media_materials/biz/shared/client/media_storage`，不得 import Hertz DTO。
- Stored Object 单 Domain 用例：`media_storage/biz/service/stored_object`；内部 handler 只做凭证、绑定和错误映射。

## 7. Tests

- Domain/Service 单元测试覆盖名称、载荷、object path、owner、only_mine、Organization 隔离和 Product Image 不变量。
- Repository 测试覆盖部分唯一索引、软删后重建、Product + Images 原子更新。
- HTTP e2e 放 `media-cli/e2e/`，通过统一网关登录后完成三类资源 CRUD；服务和基础设施由 docker-compose 启动，测试自身不管理进程。
- media_storage LocalBackend 使用临时目录测试路径逃逸、重复上传、读取和删除。
- upload workflow 使用 fake Storage client 覆盖路径确定性、用途/媒体类型/大小校验、冲突和不可用映射；Storage internal handler 覆盖缺失/错误凭证与 Namespace 越权。

## 8. Migration capability coverage

| 原能力 | 目标限界上下文 | Domain / Workflow | 状态 | 说明 |
|:---|:---|:---|:---|:---|
| Material CRUD | media_materials | asset | covered | 第一阶段已实现 |
| Virtual IP CRUD 与身份图片引用 | media_materials | virtual_ip + library_object_upload | covered | 第一阶段已实现 |
| Product CRUD 与有序图片 | media_materials | product + library_object_upload | covered | 第一阶段已实现 |
| 浏览器文件上传 | media_materials / media_storage | library_object_upload + stored_object | covered | 浏览器不直连 Storage |
| Workflow 生成产物 | media_workflow / media_materials | workflow_execution + generated_asset | external-owner | 本期不迁移，由 media_workflow 后续执行链拥有 |
| Material Binding / Reachability | media_materials | asset + reachability | deprecated-with-reason | 第一阶段明确不开放写入口，保留后续领域设计 |
| Pose / Movement Compatibility | media_materials | asset | deprecated-with-reason | 第一阶段明确排除 |
| 旧 IpMaterial | — | — | deprecated-with-reason | 用户明确暂不考虑 IP 专属素材 |
| 发布计划与统计 | — | — | deprecated-with-reason | 不属于 Materials 第一阶段 |

本期能力矩阵没有 `missing`；标记 `deprecated-with-reason` 的条目是明确排除的迁移能力，不得出现于第一阶段公开路由。
