DROP INDEX IF EXISTS idx_material_bindings_active;
ALTER TABLE material_bindings DROP COLUMN IF EXISTS bindings_end_at;
