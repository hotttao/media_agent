BEGIN;

ALTER TABLE media_materials.materials
    ADD COLUMN organization_id text,
    ADD COLUMN description text,
    ADD COLUMN prompt text,
    ADD COLUMN object_path text,
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;
UPDATE media_materials.materials
SET organization_id = 'org_legacy'
WHERE organization_id IS NULL;
ALTER TABLE media_materials.materials ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE media_materials.materials DROP CONSTRAINT IF EXISTS materials_check;
ALTER TABLE media_materials.materials
    ADD CONSTRAINT materials_payload_check
    CHECK (
        (object_path IS NOT NULL AND object_path <> '')
        OR (content IS NOT NULL AND content <> '')
        OR (url IS NOT NULL AND url <> '')
    );
DROP INDEX IF EXISTS media_materials.materials_scope_kind_name_idx;
CREATE UNIQUE INDEX materials_org_scope_kind_name_idx
    ON media_materials.materials (organization_id, scope, asset_kind, name)
    WHERE deleted_at IS NULL;
CREATE INDEX materials_org_scope_kind_idx
    ON media_materials.materials (organization_id, scope, asset_kind, updated_at DESC);

ALTER TABLE media_materials.generated_assets ADD COLUMN organization_id text;
UPDATE media_materials.generated_assets SET organization_id = 'org_legacy' WHERE organization_id IS NULL;
ALTER TABLE media_materials.generated_assets ALTER COLUMN organization_id SET NOT NULL;
DROP INDEX IF EXISTS media_materials.gen_assets_scope_kind_name_idx;
CREATE UNIQUE INDEX gen_assets_org_scope_kind_name_idx
    ON media_materials.generated_assets (organization_id, scope, asset_kind, name)
    WHERE deleted_at IS NULL;

ALTER TABLE media_materials.virtual_ips
    ADD COLUMN organization_id text,
    ADD COLUMN avatar_object_path text,
    ADD COLUMN full_body_object_path text,
    ADD COLUMN three_view_object_path text,
    ADD COLUMN nine_view_object_path text,
    ADD COLUMN persona_profile jsonb NOT NULL DEFAULT '{}'::jsonb;
UPDATE media_materials.virtual_ips
SET organization_id = 'org_legacy'
WHERE organization_id IS NULL;
ALTER TABLE media_materials.virtual_ips ALTER COLUMN organization_id SET NOT NULL;
DROP INDEX IF EXISTS media_materials.virtual_ips_scope_name_idx;
CREATE UNIQUE INDEX virtual_ips_org_scope_name_idx
    ON media_materials.virtual_ips (organization_id, scope, name)
    WHERE deleted_at IS NULL;
CREATE INDEX virtual_ips_org_scope_status_idx
    ON media_materials.virtual_ips (organization_id, scope, status);

ALTER TABLE media_materials.virtual_ip_grants ADD COLUMN organization_id text;
UPDATE media_materials.virtual_ip_grants g
SET organization_id = v.organization_id
FROM media_materials.virtual_ips v
WHERE v.id = g.virtual_ip_id AND g.organization_id IS NULL;
ALTER TABLE media_materials.virtual_ip_grants ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX virtual_ip_grants_org_team_idx
    ON media_materials.virtual_ip_grants (organization_id, team_id);

ALTER TABLE media_materials.products
    ADD COLUMN organization_id text,
    ADD COLUMN tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN external_commerce jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE media_materials.products
    ALTER COLUMN display_actions TYPE jsonb
    USING CASE
        WHEN display_actions IS NULL OR btrim(display_actions) = '' THEN '{}'::jsonb
        ELSE display_actions::jsonb
    END;
UPDATE media_materials.products SET organization_id = 'org_legacy' WHERE organization_id IS NULL;
ALTER TABLE media_materials.products ALTER COLUMN organization_id SET NOT NULL;
DROP INDEX IF EXISTS media_materials.products_scope_name_idx;
CREATE UNIQUE INDEX products_org_scope_name_idx
    ON media_materials.products (organization_id, scope, name)
    WHERE deleted_at IS NULL;
CREATE INDEX products_org_scope_status_idx
    ON media_materials.products (organization_id, scope, status);

CREATE TABLE media_materials.product_images (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    product_id      text NOT NULL REFERENCES media_materials.products(id) ON DELETE CASCADE,
    object_path     text NOT NULL,
    is_main         boolean NOT NULL DEFAULT false,
    sort_order      integer NOT NULL CHECK (sort_order >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, sort_order)
);
CREATE UNIQUE INDEX product_images_one_main_idx
    ON media_materials.product_images (product_id) WHERE is_main;
CREATE INDEX product_images_org_product_idx
    ON media_materials.product_images (organization_id, product_id, sort_order);

COMMIT;
