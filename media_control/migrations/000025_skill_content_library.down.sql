DROP TABLE skill_versions;
DROP TABLE skill_grants;
DROP INDEX skills_organization_updated;
ALTER TABLE skills DROP COLUMN organization_id,DROP COLUMN visibility,DROP COLUMN description,
 DROP COLUMN draft_checksum,DROP COLUMN published_version,DROP COLUMN source_kind,
 DROP COLUMN git_url,DROP COLUMN git_branch,DROP COLUMN git_subpath,DROP COLUMN git_commit,DROP COLUMN encrypted_git_token;
