-- =====================================================================
-- media_materials — 初始化 schema
-- =====================================================================
-- 范围: media_materials 服务自管的全部表
--   - asset_kinds        资源类别注册表（全局，无 scope）
--   - materials          curated 原料
--   - generated_assets   workflow 产物（持生成记录）
--   - material_bindings  material ↔ virtual_ip / material ↔ product
--   - products           业务产品（无 user，scope 级共享）
--   - virtual_ips        虚拟形象
--   - virtual_ip_grants  virtual_ip 资源授权
-- id: text（uuid/nanoid，service 生成）
-- =====================================================================

BEGIN;

CREATE SCHEMA IF NOT EXISTS media_materials;

-- ─────────────────────────────────────────────────────────────────────
-- 1. asset_kinds —— 资源类别注册表（全局，无 scope）
-- ─────────────────────────────────────────────────────────────────────
-- name: 业务主键（scene/pose/model_image/...），PT slot.kind / output_asset_kind 引用
-- resource_type: MATERIAL/ASSET/PRODUCT/VIRTUAL_IP/TEXT —— 决定落表
-- media_type:    IMAGE/TEXT/VIDEO
-- source_table:  materials/generated_assets/products/virtual_ips/(无)
-- status:        SYSTEM（不可删/锁列）/ENABLED/DISABLED
-- 锁列: name/resource_type/media_type/source_table 创建后不可改
CREATE TABLE IF NOT EXISTS media_materials.asset_kinds (
    name           text PRIMARY KEY,
    resource_type  text NOT NULL CHECK (resource_type IN ('MATERIAL','ASSET','PRODUCT','VIRTUAL_IP','TEXT')),
    media_type     text NOT NULL CHECK (media_type IN ('IMAGE','TEXT','VIDEO')),
    source_table   text NOT NULL,
    description    text,
    status         text NOT NULL DEFAULT 'ENABLED' CHECK (status IN ('SYSTEM','ENABLED','DISABLED')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS asset_kinds_resource_type_idx
    ON media_materials.asset_kinds (resource_type, status);
CREATE INDEX IF NOT EXISTS asset_kinds_media_type_idx
    ON media_materials.asset_kinds (media_type);

-- ─────────────────────────────────────────────────────────────────────
-- 2. materials —— curated 原料
-- ─────────────────────────────────────────────────────────────────────
-- asset_kind → asset_kinds.name (resource_type=MATERIAL)
-- url (IMAGE/VIDEO) / content (TEXT) 至少一个非空
-- visibility: private/team/public（team 求值依赖 media_auth team，后续接入）
CREATE TABLE IF NOT EXISTS media_materials.materials (
    id             text PRIMARY KEY,
    scope          text NOT NULL,
    name           text NOT NULL,
    asset_kind     text NOT NULL,
    url            text,
    content        text,
    owner_user_id  text NOT NULL,
    visibility     text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
    team_id        text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz,
    CHECK ((url IS NOT NULL) OR (content IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS materials_scope_kind_name_idx
    ON media_materials.materials (scope, asset_kind, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS materials_scope_kind_idx
    ON media_materials.materials (scope, asset_kind, updated_at DESC);
CREATE INDEX IF NOT EXISTS materials_owner_idx
    ON media_materials.materials (owner_user_id);
CREATE INDEX IF NOT EXISTS materials_team_idx
    ON media_materials.materials (team_id);

-- ─────────────────────────────────────────────────────────────────────
-- 3. generated_assets —— workflow 产物（持生成记录）
-- ─────────────────────────────────────────────────────────────────────
-- asset_kind resource_type=ASSET；= 产出该 asset 的 template.output_asset_kind
-- job_id / step_run_id → media_workflow（跨库，无 FK）
-- source_virtual_ip_id / source_product_id: 生成时的输入容器（可达性用）
-- 生成记录详情在 media_workflow.step_runs，本表只持反向链接
CREATE TABLE IF NOT EXISTS media_materials.generated_assets (
    id                   text PRIMARY KEY,
    scope                text NOT NULL,
    name                 text NOT NULL,
    asset_kind           text NOT NULL,
    url                  text,
    content              text,
    owner_user_id        text NOT NULL,
    visibility           text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
    team_id              text,
    job_id               text NOT NULL,
    step_run_id          text NOT NULL,
    template_id          text NOT NULL,
    template_version     int NOT NULL,
    source_virtual_ip_id text,
    source_product_id    text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz,
    CHECK ((url IS NOT NULL) OR (content IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS gen_assets_scope_kind_name_idx
    ON media_materials.generated_assets (scope, asset_kind, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS gen_assets_job_idx
    ON media_materials.generated_assets (job_id);
CREATE INDEX IF NOT EXISTS gen_assets_source_vip_idx
    ON media_materials.generated_assets (source_virtual_ip_id);
CREATE INDEX IF NOT EXISTS gen_assets_source_product_idx
    ON media_materials.generated_assets (source_product_id);
CREATE INDEX IF NOT EXISTS gen_assets_owner_idx
    ON media_materials.generated_assets (owner_user_id);

-- ─────────────────────────────────────────────────────────────────────
-- 4. material_bindings —— material ↔ virtual_ip / material ↔ product
-- ─────────────────────────────────────────────────────────────────────
-- INSERT 前 service 校验 IsSameScope(material.scope, owner.scope)
CREATE TABLE IF NOT EXISTS media_materials.material_bindings (
    material_id  text NOT NULL,
    owner_kind   text NOT NULL CHECK (owner_kind IN ('virtual_ip','product')),
    owner_id     text NOT NULL,
    bound_by     text NOT NULL,
    bound_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (material_id, owner_kind, owner_id)
);
CREATE INDEX IF NOT EXISTS material_bindings_owner_idx
    ON media_materials.material_bindings (owner_kind, owner_id);

-- ─────────────────────────────────────────────────────────────────────
-- 5. products —— 业务产品（无 user，scope 级共享，无 visibility）
-- ─────────────────────────────────────────────────────────────────────
-- created_by / updated_by 仅审计，不作 ACL
CREATE TABLE IF NOT EXISTS media_materials.products (
    id               text PRIMARY KEY,
    scope            text NOT NULL,
    name             text NOT NULL,
    description      text,
    target_audience text,
    product_details  text,
    display_actions  text,
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_by       text NOT NULL,
    updated_by       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS products_scope_name_idx
    ON media_materials.products (scope, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS products_scope_status_idx
    ON media_materials.products (scope, status);

-- ─────────────────────────────────────────────────────────────────────
-- 6. virtual_ips —— 虚拟形象
-- ─────────────────────────────────────────────────────────────────────
-- avatar_url/full_body_url/three_view_url/nine_view_url: IP 基础图（身份字段，非 asset）
-- visibility + grants: 资源级 ACL
CREATE TABLE IF NOT EXISTS media_materials.virtual_ips (
    id             text PRIMARY KEY,
    scope          text NOT NULL,
    name           text NOT NULL,
    description    text,
    avatar_url     text,
    full_body_url  text,
    three_view_url text,
    nine_view_url  text,
    status         text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    visibility     text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
    owner_user_id  text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS virtual_ips_scope_name_idx
    ON media_materials.virtual_ips (scope, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS virtual_ips_scope_status_idx
    ON media_materials.virtual_ips (scope, status);
CREATE INDEX IF NOT EXISTS virtual_ips_owner_idx
    ON media_materials.virtual_ips (owner_user_id);

-- ─────────────────────────────────────────────────────────────────────
-- 7. virtual_ip_grants —— virtual_ip 资源授权
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_materials.virtual_ip_grants (
    virtual_ip_id  text NOT NULL,
    team_id        text NOT NULL,
    permission     text NOT NULL CHECK (permission IN ('viewer','editor')),
    granted_by     text NOT NULL,
    granted_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (virtual_ip_id, team_id)
);
CREATE INDEX IF NOT EXISTS virtual_ip_grants_team_idx
    ON media_materials.virtual_ip_grants (team_id);

COMMIT;
