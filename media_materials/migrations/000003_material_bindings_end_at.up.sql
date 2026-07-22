-- 000003: material_bindings 加 bindings_end_at
-- owner（virtual_ip / product）软删时置 bindings_end_at = now()，绑定即时失效但保留可查；
-- 可达性 / ListByOwner 查询过滤 bindings_end_at IS NULL 取生效绑定。
-- 见 docs/media_materials/domain/0002-asset/asset.md §material_bindings。

ALTER TABLE media_materials.material_bindings ADD COLUMN bindings_end_at timestamptz;

CREATE INDEX idx_material_bindings_active
  ON media_materials.material_bindings (owner_kind, owner_id)
  WHERE bindings_end_at IS NULL;
