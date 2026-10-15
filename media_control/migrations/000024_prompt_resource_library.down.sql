DROP TABLE prompt_grants;
DROP INDEX prompts_organization_updated;
ALTER TABLE prompts DROP COLUMN visibility;
ALTER TABLE prompts DROP COLUMN organization_id;
ALTER TABLE catalog_resource_versions DROP COLUMN recorded_at;
-- Former Workspace ownership is intentionally not reconstructed from enterprise ownership.
