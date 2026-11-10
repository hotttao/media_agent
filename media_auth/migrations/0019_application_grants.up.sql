BEGIN;
CREATE TABLE media_auth.application_team_grants (
 application_id text NOT NULL REFERENCES media_auth.applications(id),
 team_id text NOT NULL REFERENCES media_auth.teams(id),
 organization_id text NOT NULL REFERENCES media_auth.organizations(id),
 scopes text[] NOT NULL,
 PRIMARY KEY(application_id,team_id)
);
ALTER TABLE media_auth.application_access_credentials ADD COLUMN team_id text NOT NULL DEFAULT '', ADD COLUMN organization_id text NOT NULL DEFAULT '', ADD COLUMN scopes text[] NOT NULL DEFAULT '{}';
COMMIT;
