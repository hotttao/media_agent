# media_materials 实现规格

> 素材库 + 资源类别注册表。管理 material（curated 原料）/ generated_asset（workflow 产物）/ product / virtual_ip / asset_kind 注册表。
>
> media_workflow 不再调 materials 做"解析"——bundle 自包含，由 caller（编排层 / BFF）从本服务查数据组装。本服务只暴露 CRUD + List + `BatchGetAssetKind` + `UpsertGeneratedAsset`。
>
> 领域规则见 `docs/media_materials/domain/*`。

## 1. 边界

- **本服务拥有**：asset_kind 注册表、material、generated_asset、product、virtual_ip、material_bindings。
- **不拥有**：template / workflow / job（归 media_workflow）。
- **id 风格**：text（uuid/nanoid，service 生成）。
- **scope**：material / generated_asset / product / virtual_ip 持 scope；asset_kind **全局**无 scope。
- **visibility**：material / generated_asset / virtual_ip = private/team/public（+ grants）；product 无 user 概念，无 visibility。

## 2. IDL

### 2.1 asset_kind.proto

```protobuf
syntax = "proto3";
package media_materials.asset_kind;
import "api.proto";

enum ResourceType { RESOURCE_TYPE_UNSPECIFIED = 0; MATERIAL = 1; ASSET = 2; PRODUCT = 3; VIRTUAL_IP = 4; TEXT = 5; }
enum MediaType { MEDIA_TYPE_UNSPECIFIED = 0; IMAGE = 1; TEXT = 2; VIDEO = 3; }
enum AssetKindStatus { AKS_UNSPECIFIED = 0; SYSTEM = 1; ENABLED = 2; DISABLED = 3; }

message AssetKind {
  string name = 1;
  ResourceType resource_type = 2;
  MediaType media_type = 3;
  string source_table = 4;
  string description = 5;
  AssetKindStatus status = 6;
  int64 created_at = 7;
  int64 updated_at = 8;
}

message RegisterAssetKindReq {
  string name = 1 [(api.body)="name"];
  ResourceType resource_type = 2 [(api.body)="resource_type"];
  MediaType media_type = 3 [(api.body)="media_type"];
  string source_table = 4 [(api.body)="source_table"];
  string description = 5 [(api.body)="description"];
}
message GetAssetKindReq { string name = 1 [(api.path)="name"]; }
message ListAssetKindsReq {
  string resource_type = 1 [(api.query)="resource_type"];
  string media_type = 2 [(api.query)="media_type"];
  string status = 3 [(api.query)="status"];
  int32 page = 4; int32 page_size = 5;
}
message ListAssetKindsResp { repeated AssetKind items = 1; int32 total = 2; }
message UpdateAssetKindReq { string name = 1 [(api.path)="name"]; string description = 2 [(api.body)="description"]; string status = 3 [(api.body)="status"]; }
// BatchGetAssetKind：被 media_workflow 校验期调用
message BatchGetAssetKindReq { repeated string names = 1; }
message BatchGetAssetKindResp { repeated AssetKind kinds = 1; repeated string not_found = 2; }

service AssetKindService {
  rpc RegisterAssetKind(RegisterAssetKindReq) returns (AssetKind) { option (api.post) = "/api/v1/asset-kinds"; }
  rpc GetAssetKind(GetAssetKindReq) returns (AssetKind) { option (api.get) = "/api/v1/asset-kinds/:name"; }
  rpc ListAssetKinds(ListAssetKindsReq) returns (ListAssetKindsResp) { option (api.get) = "/api/v1/asset-kinds"; }
  rpc UpdateAssetKind(UpdateAssetKindReq) returns (AssetKind) { option (api.put) = "/api/v1/asset-kinds/:name"; }
  rpc DeleteAssetKind(GetAssetKindReq) returns (AssetKind) { option (api.delete) = "/api/v1/asset-kinds/:name"; }
  rpc BatchGetAssetKind(BatchGetAssetKindReq) returns (BatchGetAssetKindResp) {}  // 内部 RPC，不暴露 HTTP
}
```

### 2.2 material.proto

```protobuf
syntax = "proto3";
package media_materials.material;
import "api.proto";

message Material {
  string id = 1;
  string scope = 2;
  string name = 3;              // 业务名
  string asset_kind = 4;        // → asset_kinds.name (resource_type=MATERIAL)
  string url = 5;               // IMAGE/VIDEO
  string content = 6;           // TEXT
  string owner_user_id = 7;
  string visibility = 8;        // private/team/public
  string team_id = 9;
  int64 created_at = 10;
  int64 updated_at = 11;
}

message CreateMaterialReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2 [(api.body)="name"];
  string asset_kind = 3 [(api.body)="asset_kind"];
  string url = 4 [(api.body)="url"];
  string content = 5 [(api.body)="content"];
  string owner_user_id = 6 [(api.query)="owner_user_id"];
  string visibility = 7 [(api.body)="visibility"];
  string team_id = 8 [(api.body)="team_id"];
}
message ListMaterialsReq {
  string scope = 1 [(api.query)="scope"];
  string asset_kind = 2 [(api.query)="asset_kind"];
  string owner = 3 [(api.query)="owner"];
  string visibility = 4 [(api.query)="visibility"];
  int32 page = 5; int32 page_size = 6;
}
message ListMaterialsResp { repeated Material items = 1; int32 total = 2; }
// 按 owner（virtual_ip / product）列出 material——caller 组装 bundle 用
message ListMaterialsByOwnerReq {
  string owner_kind = 1 [(api.query)="owner_kind"];   // virtual_ip / product
  string owner_id = 2 [(api.query)="owner_id"];
}
message ListMaterialsByOwnerResp { repeated Material items = 1; }

message AddBindingReq {
  string material_id = 1 [(api.path)="material_id"];
  string owner_kind = 2 [(api.body)="owner_kind"];
  string owner_id = 3 [(api.body)="owner_id"];
  string bound_by = 4 [(api.query)="bound_by"];
}

service MaterialService {
  rpc CreateMaterial(CreateMaterialReq) returns (Material) { option (api.post) = "/api/v1/materials"; }
  rpc GetMaterial(GetMaterialReq) returns (Material) { option (api.get) = "/api/v1/materials/:id"; }
  rpc ListMaterials(ListMaterialsReq) returns (ListMaterialsResp) { option (api.get) = "/api/v1/materials"; }
  rpc ListMaterialsByOwner(ListMaterialsByOwnerReq) returns (ListMaterialsByOwnerResp) { option (api.get) = "/api/v1/materials/by-owner"; }
  rpc UpdateMaterial(UpdateMaterialReq) returns (Material) { option (api.put) = "/api/v1/materials/:id"; }
  rpc DeleteMaterial(GetMaterialReq) returns (Material) { option (api.delete) = "/api/v1/materials/:id"; }
  rpc AddMaterialBinding(AddBindingReq) returns (Material) { option (api.post) = "/api/v1/materials/:material_id/bindings"; }
  rpc RemoveMaterialBinding(RemoveBindingReq) returns (Material) { option (api.delete) = "/api/v1/materials/:material_id/bindings/:owner_kind/:owner_id"; }
}
```

### 2.3 generated_asset.proto

```protobuf
syntax = "proto3";
package media_materials.generated_asset;
import "api.proto";

message GeneratedAsset {
  string id = 1;
  string scope = 2;
  string name = 3;
  string asset_kind = 4;        // resource_type=ASSET
  string url = 5;
  string content = 6;           // TEXT 产物
  string owner_user_id = 7;     // = job.submitted_by
  string visibility = 8;
  string team_id = 9;
  string job_id = 10;           // → media_workflow.jobs.id
  string step_run_id = 11;      // → media_workflow.step_runs.id
  string template_id = 12;
  int32 template_version = 13;
  string source_virtual_ip_id = 14;
  string source_product_id = 15;
  int64 created_at = 16;
}

message GetGeneratedAssetReq { string id = 1 [(api.path)="id"]; }
message ListGeneratedAssetsReq {
  string scope = 1 [(api.query)="scope"];
  string asset_kind = 2 [(api.query)="asset_kind"];
  string owner = 3 [(api.query)="owner"];
  string job_id = 4 [(api.query)="job_id"];
  string source_virtual_ip_id = 5 [(api.query)="source_virtual_ip_id"];
  int32 page = 6; int32 page_size = 7;
}
message ListGeneratedAssetsResp { repeated GeneratedAsset items = 1; int32 total = 2; }
// 按 source（virtual_ip / product）列出——caller 组装 bundle 用
message ListGeneratedAssetsBySourceReq { string source_kind = 1; string source_id = 2; }
message ListGeneratedAssetsBySourceResp { repeated GeneratedAsset items = 1; }

// UpsertGeneratedAsset：被 media_workflow execution 期调用
message UpsertGeneratedAssetReq {
  string scope = 1;
  string asset_kind = 2;        // = template.output_asset_kind
  string name = 3;
  string url = 4;
  string content = 5;
  string owner_user_id = 6;     // = job.submitted_by
  string visibility = 7;        // 默认 private
  string job_id = 8;
  string step_run_id = 9;
  string template_id = 10;
  int32 template_version = 11;
  string source_virtual_ip_id = 12;
  string source_product_id = 13;
}
message UpsertGeneratedAssetResp { string id = 1; }

service GeneratedAssetService {
  rpc GetGeneratedAsset(GetGeneratedAssetReq) returns (GeneratedAsset) { option (api.get) = "/api/v1/generated-assets/:id"; }
  rpc ListGeneratedAssets(ListGeneratedAssetsReq) returns (ListGeneratedAssetsResp) { option (api.get) = "/api/v1/generated-assets"; }
  rpc ListGeneratedAssetsBySource(ListGeneratedAssetsBySourceReq) returns (ListGeneratedAssetsBySourceResp) { option (api.get) = "/api/v1/generated-assets/by-source"; }
  rpc DeleteGeneratedAsset(GetGeneratedAssetReq) returns (GeneratedAsset) { option (api.delete) = "/api/v1/generated-assets/:id"; }
  rpc UpsertGeneratedAsset(UpsertGeneratedAssetReq) returns (UpsertGeneratedAssetResp) {}  // 内部 RPC
}
```

### 2.4 product.proto

```protobuf
syntax = "proto3";
package media_materials.product;
import "api.proto";

message Product {
  string id = 1;
  string scope = 2;
  string name = 3;
  string description = 4;
  string target_audience = 5;
  string product_details = 6;
  string display_actions = 7;
  string status = 8;            -- active/archived
  string created_by = 9;
  string updated_by = 10;
  int64 created_at = 11;
  int64 updated_at = 12;
}

message CreateProductReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2; string description = 3; string target_audience = 4;
  string product_details = 5; string display_actions = 6;
  string created_by = 7 [(api.query)="created_by";
}
message ListProductsReq { string scope = 1; string name = 2; string status = 3; int32 page = 4; int32 page_size = 5; }
message ListProductsResp { repeated Product items = 1; int32 total = 2; }

service ProductService {
  rpc CreateProduct(CreateProductReq) returns (Product) { option (api.post) = "/api/v1/products"; }
  rpc GetProduct(GetProductReq) returns (Product) { option (api.get) = "/api/v1/products/:id"; }
  rpc ListProducts(ListProductsReq) returns (ListProductsResp) { option (api.get) = "/api/v1/products"; }
  rpc UpdateProduct(UpdateProductReq) returns (Product) { option (api.put) = "/api/v1/products/:id"; }
  rpc ArchiveProduct(GetProductReq) returns (Product) { option (api.post) = "/api/v1/products/:id/archive"; }
  rpc DeleteProduct(GetProductReq) returns (Product) { option (api.delete) = "/api/v1/products/:id"; }
}
```

> product 无 user 概念、无 visibility/grants——任意已认证 user 可 CRUD（created_by/updated_by 仅审计）。product ↔ material 绑定复用 MaterialService.AddMaterialBinding（owner_kind=product）。

### 2.5 virtual_ip.proto

```protobuf
syntax = "proto3";
package media_materials.virtual_ip;
import "api.proto";

message VirtualIp {
  string id = 1;
  string scope = 2;
  string name = 3;
  string description = 4;
  string avatar_url = 5;
  string full_body_url = 6;
  string three_view_url = 7;
  string nine_view_url = 8;
  string status = 9;            -- active/archived
  string visibility = 10;       -- private/team/public
  string owner_user_id = 11;
  int64 created_at = 12;
  int64 updated_at = 13;
}

message CreateVirtualIpReq {
  string scope = 1; string name = 2; string description = 3;
  string avatar_url = 4; string full_body_url = 5; string three_view_url = 6; string nine_view_url = 7;
  string owner_user_id = 8; string visibility = 9;
}
message GrantReq { string id = 1; string team_id = 2; string permission = 3; }
message ChangeVisibilityReq { string id = 1; string visibility = 2; }

service VirtualIpService {
  rpc CreateVirtualIp(CreateVirtualIpReq) returns (VirtualIp) { option (api.post) = "/api/v1/virtual-ips"; }
  rpc GetVirtualIp(GetVirtualIpReq) returns (VirtualIp) { option (api.get) = "/api/v1/virtual-ips/:id"; }
  rpc ListVirtualIps(ListVirtualIpsReq) returns (ListVirtualIpsResp) { option (api.get) = "/api/v1/virtual-ips"; }
  rpc UpdateVirtualIp(UpdateVirtualIpReq) returns (VirtualIp) { option (api.put) = "/api/v1/virtual-ips/:id"; }
  rpc ArchiveVirtualIp(GetVirtualIpReq) returns (VirtualIp) { option (api.post) = "/api/v1/virtual-ips/:id/archive"; }
  rpc DeleteVirtualIp(GetVirtualIpReq) returns (VirtualIp) { option (api.delete) = "/api/v1/virtual-ips/:id"; }
  rpc GrantVirtualIp(GrantReq) returns (VirtualIp) { option (api.post) = "/api/v1/virtual-ips/:id/grants"; }
  rpc RevokeVirtualIp(GrantReq) returns (VirtualIp) { option (api.delete) = "/api/v1/virtual-ips/:id/grants/:team_id"; }
  rpc ChangeVirtualIpVisibility(ChangeVisibilityReq) returns (VirtualIp) { option (api.post) = "/api/v1/virtual-ips/:id/visibility"; }
}
```

## 3. 数据库

```sql
-- asset_kind 注册表（全局，无 scope）
CREATE TABLE asset_kinds (
  name text PRIMARY KEY,
  resource_type text NOT NULL CHECK (resource_type IN ('MATERIAL','ASSET','PRODUCT','VIRTUAL_IP','TEXT')),
  media_type text NOT NULL CHECK (media_type IN ('IMAGE','TEXT','VIDEO')),
  source_table text NOT NULL,
  description text,
  status text NOT NULL DEFAULT 'ENABLED' CHECK (status IN ('SYSTEM','ENABLED','DISABLED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_asset_kinds_resource_type ON asset_kinds (resource_type, status);
CREATE INDEX idx_asset_kinds_media_type ON asset_kinds (media_type);

-- curated 原料
CREATE TABLE materials (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  asset_kind text NOT NULL,       -- → asset_kinds.name, resource_type=MATERIAL
  url text,                       -- IMAGE/VIDEO
  content text,                   -- TEXT
  owner_user_id text NOT NULL,
  visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
  team_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK ((url IS NOT NULL) OR (content IS NOT NULL))  -- 至少一个
);
CREATE UNIQUE INDEX uk_materials_scope_kind_name ON materials (scope, asset_kind, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_materials_scope_kind ON materials (scope, asset_kind, updated_at DESC);
CREATE INDEX idx_materials_owner ON materials (owner_user_id);
CREATE INDEX idx_materials_team ON materials (team_id);

-- workflow 产物
CREATE TABLE generated_assets (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  asset_kind text NOT NULL,       -- resource_type=ASSET
  url text,
  content text,                   -- TEXT 产物
  owner_user_id text NOT NULL,    -- = job.submitted_by
  visibility text NOT NULL DEFAULT 'private',
  team_id text,
  job_id text NOT NULL,           -- → media_workflow.jobs.id（跨库，无 FK）
  step_run_id text NOT NULL,      -- → media_workflow.step_runs.id
  template_id text NOT NULL,
  template_version int NOT NULL,
  source_virtual_ip_id text,
  source_product_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK ((url IS NOT NULL) OR (content IS NOT NULL))
);
CREATE UNIQUE INDEX uk_generated_assets_scope_kind_name ON generated_assets (scope, asset_kind, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_generated_assets_job ON generated_assets (job_id);
CREATE INDEX idx_generated_assets_source_vip ON generated_assets (source_virtual_ip_id);
CREATE INDEX idx_generated_assets_source_product ON generated_assets (source_product_id);
CREATE INDEX idx_generated_assets_owner ON generated_assets (owner_user_id);

-- material ↔ virtual_ip / material ↔ product
CREATE TABLE material_bindings (
  material_id text NOT NULL,
  owner_kind text NOT NULL CHECK (owner_kind IN ('virtual_ip','product')),
  owner_id text NOT NULL,
  bound_by text NOT NULL,
  bound_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (material_id, owner_kind, owner_id)
);
CREATE INDEX idx_material_bindings_owner ON material_bindings (owner_kind, owner_id);

CREATE TABLE products (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  description text,
  target_audience text,
  product_details text,
  display_actions text,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  created_by text NOT NULL,
  updated_by text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX uk_products_scope_name ON products (scope, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_scope_status ON products (scope, status);

CREATE TABLE virtual_ips (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  description text,
  avatar_url text,
  full_body_url text,
  three_view_url text,
  nine_view_url text,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
  owner_user_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX uk_virtual_ips_scope_name ON virtual_ips (scope, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_virtual_ips_scope_status ON virtual_ips (scope, status);
CREATE INDEX idx_virtual_ips_owner ON virtual_ips (owner_user_id);

CREATE TABLE virtual_ip_grants (
  virtual_ip_id text NOT NULL,
  team_id text NOT NULL,
  permission text NOT NULL CHECK (permission IN ('viewer','editor')),
  granted_by text NOT NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (virtual_ip_id, team_id)
);
CREATE INDEX idx_virtual_ip_grants_team ON virtual_ip_grants (team_id);
```

## 4. 关键行为

### 4.1 visibility 校验（resource ACL）
material / generated_asset / virtual_ip 的每次读写，service 入口推导 `MyPermission(user, resource)`（owner ∨ grants ∨ public），按 `docs/media_auth/CONTEXT.md#资源级-acl` 决定 viewer/editor/owner 可执行的操作。product 不做 user 校验。

### 4.2 asset_kind 校验
- UpsertMaterial：asset_kind 须已注册、resource_type=MATERIAL、status∈{SYSTEM,ENABLED}。
- UpsertGeneratedAsset：asset_kind 须 resource_type=ASSET。
- TEXT kind：url 可空、content 必填；IMAGE/VIDEO kind：url 必填、content 可空。（DB CHECK 兜底）

### 4.3 material_bindings 同 scope
INSERT 前 service 比较 `material.scope == owner.scope`（owner = virtual_ip / product），不一致 reject。

### 4.4 UpsertGeneratedAsset（被 media_workflow 调）
- 生成记录字段（job_id / step_run_id / template_id / template_version / source_*_id）由 workflow 填入，不可改。
- scope = workflow.scope；owner_user_id = job.submitted_by；visibility 默认 private。

## 5. 配置

```yaml
# conf/local.yaml
supported_scopes: [ai_clothing, ai_guoxue]
```
> asset_kind 注册表通过 seed 迁移初始化 SYSTEM kinds（scene/pose/movement/model_image/style_image/first_frame/ai_video/product_image/...）；ENABLED kinds 由 admin 通过 RegisterAssetKind 增。

## 6. 与 media_workflow 的契约

| RPC | 调用方 | 时机 | 用途 |
|:---|:---|:---|:---|
| `BatchGetAssetKind(names)` | media_workflow | Workflow 校验期 | 校验 slot.name / output_asset_kind 已注册、resource_type/media_type 匹配 |
| `UpsertGeneratedAsset(...)` | media_workflow | Execution 期 step 产出 | 落 generated_asset + 生成记录 |

caller（编排层 / BFF）用本服务的 CRUD + ListByOwner/ListBySource 组装 JobInput bundle，再向 media_workflow 提交 SubmitJob。

### bundle 组装映射（caller 侧）

| bundle 字段 | 来源 |
|:---|:---|
| `MaterialData.name` | `material.asset_kind`（**类型**，非 materials.name 业务名） |
| `MaterialData.url` / `content` | `material.url` / `material.content` |
| `AssetData.name` | `generated_asset.asset_kind` |
| `AssetData.url` / `content` | `generated_asset.url` / `generated_asset.content` |
| `ProductData.*` | `product.*`（同名字段） |
| `VirtualIpData.*` | `virtual_ip.*`（avatar_url / full_body_url / ...） |

> `materials.name` / `generated_assets.name` 是业务名（管理 UI 用），**不进 bundle**；bundle 的 `name` 恒等于 `asset_kind`（类型），是 slot 的选择键。

## 7. 未定义的 boilerplate message

`GetMaterialReq{string id}` / `UpdateMaterialReq` / `RemoveBindingReq` / `GetProductReq` / `UpdateProductReq` / `GetVirtualIpReq` / `UpdateVirtualIpReq` / `ListVirtualIpsReq/Resp` 等标准 CRUD 入参出参未逐一展开，实现时按 `{id}` path 入参 + body 字段 + 分页的常规模式补全。
