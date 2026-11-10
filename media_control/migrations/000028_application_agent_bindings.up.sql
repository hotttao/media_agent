BEGIN;
CREATE TABLE application_agent_bindings (
 application_id text NOT NULL,user_id text NOT NULL,team_id text NOT NULL,name text NOT NULL,
 workspace_id text NOT NULL UNIQUE REFERENCES workspaces(id),digest text NOT NULL,
 PRIMARY KEY(application_id,user_id,team_id,name)
);
COMMIT;
