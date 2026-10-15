ALTER TABLE skills ADD COLUMN organization_id text;
ALTER TABLE skills ADD COLUMN visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public'));
ALTER TABLE skills ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN draft_checksum text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN published_version bigint NOT NULL DEFAULT 0;
ALTER TABLE skills ADD COLUMN source_kind text NOT NULL DEFAULT 'native' CHECK(source_kind IN ('native','upload','git'));
ALTER TABLE skills ADD COLUMN git_url text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN git_branch text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN git_subpath text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN git_commit text NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN encrypted_git_token bytea;
ALTER TABLE skills DROP CONSTRAINT IF EXISTS skills_workspace_id_fkey;
-- Secrets never enter Catalog history, including encrypted Git credentials.
CREATE OR REPLACE FUNCTION capture_catalog_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 snapshot := to_jsonb(NEW) - 'created_at' - 'updated_at' - 'encrypted_git_token';
 IF TG_OP = 'UPDATE' AND NEW.version <= OLD.version AND snapshot <> (to_jsonb(OLD)-'created_at'-'updated_at'-'encrypted_git_token') THEN
  RAISE EXCEPTION 'catalog version must increase';
 END IF;
 INSERT INTO catalog_resource_versions(kind,resource_id,version,owner_user_id,workspace_id,payload)
 VALUES (TG_ARGV[0],NEW.id,NEW.version,NEW.owner_user_id,NEW.workspace_id,snapshot) ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TABLE skill_grants (
 skill_id text NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
 team_id text NOT NULL, permission text NOT NULL CHECK(permission IN ('viewer','editor')),
 PRIMARY KEY(skill_id,team_id)
);
CREATE TABLE skill_versions (
 skill_id text NOT NULL REFERENCES skills(id),version bigint NOT NULL CHECK(version>0),
 checksum text NOT NULL, slug text NOT NULL, description text NOT NULL,
 published_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(skill_id,version)
);
INSERT INTO skill_versions(skill_id,version,checksum,slug,description,published_by,created_at)
SELECT resource_id,version,payload->>'checksum','','',owner_user_id,COALESCE(recorded_at,now())
FROM catalog_resource_versions WHERE kind='media_control.skill'
 AND payload->>'checksum' ~ '^[0-9a-f]{64}$' AND payload->>'content_path'='sha256/'||(payload->>'checksum');
UPDATE skills s SET organization_id=w.organization_id,workspace_id=NULL,version=s.version+1,updated_at=now()
FROM workspaces w WHERE s.workspace_id=w.id AND w.organization_id IS NOT NULL AND w.organization_id<>'';
UPDATE skills s SET published_version=v.version,draft_checksum=s.checksum,version=s.version+1,updated_at=now()
FROM (SELECT skill_id,max(version) version FROM skill_versions GROUP BY skill_id) v WHERE v.skill_id=s.id;
CREATE INDEX skills_organization_updated ON skills(organization_id,updated_at DESC,id);
