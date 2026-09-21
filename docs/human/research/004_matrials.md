# Materials 第一阶段需求对齐

> 文件名沿用既有编号；正文统一使用正确术语 `Materials`。

## 本期目标

第一阶段只实现以下三个核心对象的增删改查：

- Material
- Virtual IP
- Product

本期明确不实现：

- Workflow 提交、执行、编译或审批；
- Material 与 Virtual IP/Product 的显式 Binding；
- Virtual IP 专属素材 `IpMaterial`；
- Product 生成链路、发布计划和发布统计；
- Pose 与 Movement 的兼容关系维护。

已有 DDD 中描述的 Workflow、Binding 和 Generated Asset 生成链路属于后续能力，不能进入本期 IDL、Migration、Service 或测试验收范围。

## Scope 语义

Scope 是 Organization 内的业务品类限定，例如某个 Virtual IP 主要从事的带货品类。

- Virtual IP、Material、Product 都必须属于一个 Organization 和一个 Scope，在企业内共享，不属于单个 Workspace。
- 同一 Organization、同一 Scope 表示三类资源处在同一个可组合业务集合中。
- 当前不建立显式 Binding；列表和选择器通过 Organization + Scope 过滤相关资源。
- Scope 不是租户边界，也不是旧 Movement Material 中“适用服装范围”的含义。
- Organization 和 Scope 创建后不可修改；跨 Organization 或跨 Scope 引用一律禁止。
- Workspace 只是后续生产流程的运行上下文；它通过所属 Team 反查 Organization 后使用企业资源，不复制资源记录。

## 图片与 Media Storage

图片二进制统一由 `media_storage` 微服务保存。它对外提供稳定接口，底层可以替换为：

- 本地文件系统（当前开发和测试）；
- MinIO；
- 阿里云 OSS 或其他云对象存储。

`media_materials` 不感知 provider、bucket、域名或签名 URL，只保存 Storage 返回的逻辑 `object_path`。

本期不在 CRUD 事务内编排跨服务上传，采用两步式调用：

1. 客户端先调用 `media_materials` 的 Library Object Upload 接口；`media_materials` 校验用户与 Organization Membership、生成 Object Path，再以服务身份调用 `media_storage`，返回 `object_path`；
2. 客户端调用 Material / Virtual IP / Product CRUD，把 `object_path` 写入业务记录。

浏览器不直接调用 `media_storage`，Storage 也不接收最终用户身份。读取图片必须先由拥有业务语义的服务按 Material / Virtual IP / Product 权限完成授权，再代理内容或签发短期地址；不得开放仅凭任意 `object_path` 读取的公共接口。删除业务记录不直接删除 Storage 对象，避免共享引用和失败补偿问题；对象保留与垃圾回收归 `media_storage`。

## Owner 与默认筛选

- 用户上传的 Material 和 Workflow 生成的 Generated Asset 都必须记录 `owner_user_id`。
- Material 列表默认 `only_mine=true`，即只返回当前 Principal 拥有的素材。
- 用户显式传 `only_mine=false` 时，才按 visibility、Team Grant、Organization 和 Scope 返回其他可见素材。
- Principal 和 Organization 必须来自可信认证上下文，不能由请求 body 覆盖；Principal 必须是该 Organization 的 active member。
- Product 沿用已确认规则：没有资源级 owner/visibility/grants；通过 Organization Membership 校验后在 Scope 内共享。
- Virtual IP 仍有 owner、visibility 和 Team Grant。

## 唯一键与不可变量

软删记录不占用业务唯一键；所有业务唯一键必须包含 Organization，禁止不同公司相互冲突。

| 对象 | 业务唯一键 | 说明 |
|:---|:---|:---|
| Material | `(organization_id, scope, asset_kind, name)` | 相同企业、品类和素材类别下名称唯一 |
| Generated Asset | `(organization_id, scope, asset_kind, name)` | 为后续生成链路保留；本期不实现创建流程 |
| Virtual IP | `(organization_id, scope, name)` | 同一企业业务品类下 IP 名唯一 |
| Product | `(organization_id, scope, name)` | 同一企业业务品类下产品名唯一 |
| Product Image | `(product_id, sort_order)` | 同一 Product 内顺序唯一，且至多一张主图 |

写接口遇到唯一键冲突必须返回稳定的 `already_exists` 业务错误，不允许把数据库错误直接暴露给客户端。

## 三个聚合的最小字段

### Material

- Organization、Scope、Asset Kind；
- name、description、prompt；
- `object_path` 或内联 `content`，至少一个存在；
- tags / metadata；
- owner、visibility、team；
- created_at、updated_at、deleted_at。

### Virtual IP

- Organization、Scope、name、description；
- avatar/full-body/three-view/nine-view 的 `object_path`；
- Persona Profile；
- owner、visibility、Team Grant、status；
- created_at、updated_at、deleted_at。

### Product

- Organization、Scope、name、description；
- target audience、product details、display actions、tags；
- External Commerce Reference；
- 有序 Product Image 集合；
- status 与审计字段；
- created_at、updated_at、deleted_at。

## 验收边界

- 三个对象的 CRUD、分页、条件筛选和软删均可用；
- Material 默认只看自己的素材，可显式查看其他有权素材；
- 所有查询和唯一键均受 Organization + Scope 限制；
- 图片通过 `media_storage` 接口上传，本地存储适配器可完成测试；
- 数据库和 API 不出现完整存储 URL、provider 或 bucket；
- 本期不存在 Workflow、Binding、生成、发布相关接口或后台任务。
