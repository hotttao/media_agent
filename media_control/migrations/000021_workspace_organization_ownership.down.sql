ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_organization_team_pair_check;
DROP INDEX IF EXISTS idx_workspaces_team_updated;
ALTER TABLE workspaces
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS team_id,
    DROP COLUMN IF EXISTS organization_id;
