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

上传 HTTP 边界尚未定稿：现有 media_storage DDD 要求只接受服务身份并由业务服务生成 Object Path；“浏览器直接上传”则要求 media_storage 接受最终用户身份与 Organization 授权。确定其中一种后，才固化 IDL、网关路由和跨服务 Workflow；在此之前不暴露上传/读取 HTTP 接口。

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
- 本期没有 `biz/workflow` 新实现。

## 7. Tests

- Domain/Service 单元测试覆盖名称、载荷、object path、owner、only_mine、Organization 隔离和 Product Image 不变量。
- Repository 测试覆盖部分唯一索引、软删后重建、Product + Images 原子更新。
- HTTP e2e 放 `media-cli/e2e/`，通过统一网关登录后完成三类资源 CRUD；服务和基础设施由 docker-compose 启动，测试自身不管理进程。
- media_storage LocalBackend 使用临时目录测试路径逃逸、重复上传、读取和删除。
