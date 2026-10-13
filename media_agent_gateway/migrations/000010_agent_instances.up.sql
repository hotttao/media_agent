CREATE TABLE agent_instances (
 environment_id text NOT NULL, workspace_id text NOT NULL, replica_id text NOT NULL,
 agent_config_id text NOT NULL, incarnation_id text NOT NULL, resource_uid text NOT NULL,
 credential_hash text NOT NULL DEFAULT '',
 endpoint text NOT NULL, admission text NOT NULL CHECK(admission IN ('active','draining')),
 version bigint NOT NULL DEFAULT 1, drain_operation_id text, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(environment_id,workspace_id,replica_id)
);
CREATE INDEX agent_instances_workspace ON agent_instances(workspace_id,agent_config_id);
