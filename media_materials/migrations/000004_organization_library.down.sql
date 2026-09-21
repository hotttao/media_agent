BEGIN;

DROP TABLE IF EXISTS media_materials.product_images;

DROP INDEX IF EXISTS media_materials.products_org_scope_status_idx;
DROP INDEX IF EXISTS media_materials.products_org_scope_name_idx;
CREATE UNIQUE INDEX products_scope_name_idx
    ON media_materials.products (scope, name) WHERE deleted_at IS NULL;
ALTER TABLE media_materials.products
    ALTER COLUMN display_actions TYPE text USING display_actions::text,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS external_commerce,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS media_materials.virtual_ip_grants_org_team_idx;
ALTER TABLE media_materials.virtual_ip_grants DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS media_materials.virtual_ips_org_scope_status_idx;
DROP INDEX IF EXISTS media_materials.virtual_ips_org_scope_name_idx;
CREATE UNIQUE INDEX virtual_ips_scope_name_idx
    ON media_materials.virtual_ips (scope, name) WHERE deleted_at IS NULL;
ALTER TABLE media_materials.virtual_ips
    DROP COLUMN IF EXISTS persona_profile,
    DROP COLUMN IF EXISTS nine_view_object_path,
    DROP COLUMN IF EXISTS three_view_object_path,
    DROP COLUMN IF EXISTS full_body_object_path,
    DROP COLUMN IF EXISTS avatar_object_path,
    DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS media_materials.gen_assets_org_scope_kind_name_idx;
CREATE UNIQUE INDEX gen_assets_scope_kind_name_idx
    ON media_materials.generated_assets (scope, asset_kind, name) WHERE deleted_at IS NULL;
ALTER TABLE media_materials.generated_assets DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS media_materials.materials_org_scope_kind_idx;
DROP INDEX IF EXISTS media_materials.materials_org_scope_kind_name_idx;
CREATE UNIQUE INDEX materials_scope_kind_name_idx
    ON media_materials.materials (scope, asset_kind, name) WHERE deleted_at IS NULL;
ALTER TABLE media_materials.materials
    DROP CONSTRAINT IF EXISTS materials_payload_check,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS object_path,
    DROP COLUMN IF EXISTS prompt,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS organization_id;
ALTER TABLE media_materials.materials
    ADD CONSTRAINT materials_check CHECK ((url IS NOT NULL) OR (content IS NOT NULL));

COMMIT;
