ALTER TABLE prompts ADD COLUMN organization_id text;
ALTER TABLE prompts ADD COLUMN visibility text NOT NULL DEFAULT 'private'
    CHECK (visibility IN ('private','team','public'));
-- Library resources must survive deletion of their former Workspace.
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_workspace_id_fkey;
ALTER TABLE catalog_resource_versions ADD COLUMN recorded_at timestamptz;
ALTER TABLE catalog_resource_versions ALTER COLUMN recorded_at SET DEFAULT now();
-- Only authoritative Workspace ownership can be migrated automatically.
-- Unassigned legacy rows stay private and require explicit organization assignment.
UPDATE prompts p SET organization_id=w.organization_id, workspace_id=NULL,
    version=p.version+1, updated_at=now()
FROM workspaces w WHERE p.workspace_id=w.id AND w.organization_id IS NOT NULL AND w.organization_id<>'';
CREATE INDEX prompts_organization_updated ON prompts(organization_id,updated_at DESC,id);
CREATE TABLE prompt_grants (
    prompt_id text NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    team_id text NOT NULL, permission text NOT NULL CHECK(permission IN ('viewer','editor')),
    PRIMARY KEY(prompt_id,team_id)
);
