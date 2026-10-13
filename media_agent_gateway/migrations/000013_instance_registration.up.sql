ALTER TABLE agent_instances ADD resource_generation text NOT NULL DEFAULT '',
 ADD resource_name text NOT NULL DEFAULT '', ADD namespace text NOT NULL DEFAULT '',
 ADD credential_key_id text NOT NULL DEFAULT '', ADD agent_type text NOT NULL DEFAULT '';
CREATE TABLE agent_instance_history (LIKE agent_instances INCLUDING DEFAULTS);
ALTER TABLE agent_instance_history ADD PRIMARY KEY(environment_id,workspace_id,replica_id,incarnation_id);
INSERT INTO agent_instance_history SELECT * FROM agent_instances;

CREATE UNIQUE INDEX instance_history_incarnation ON agent_instance_history(workspace_id,agent_config_id,replica_id,incarnation_id);
