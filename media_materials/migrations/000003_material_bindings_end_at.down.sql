DROP INDEX IF EXISTS media_materials.idx_material_bindings_active;
ALTER TABLE media_materials.material_bindings DROP COLUMN IF EXISTS bindings_end_at;
