ALTER TABLE workspaces
    ADD COLUMN IF NOT EXISTS organization_id text,
    ADD COLUMN IF NOT EXISTS team_id text,
    ADD COLUMN IF NOT EXISTS created_by text;

CREATE INDEX IF NOT EXISTS idx_workspaces_team_updated
    ON workspaces (team_id, updated_at DESC)
    WHERE team_id IS NOT NULL;

ALTER TABLE workspaces
    ADD CONSTRAINT workspaces_organization_team_pair_check
    CHECK ((organization_id IS NULL AND team_id IS NULL) OR
           (organization_id IS NOT NULL AND team_id IS NOT NULL)) NOT VALID;

COMMENT ON COLUMN workspaces.owner_user_id IS
    'legacy ownership only; new organization-scoped writes use created_by and team_id';
