ALTER TABLE media_agent_gateway.agent_sessions
 ADD environment_id text NOT NULL DEFAULT '', ADD replica_id text NOT NULL DEFAULT '',
 ADD incarnation_id text NOT NULL DEFAULT '', ADD binding_version bigint NOT NULL DEFAULT 0,
 ADD binding_phase text NOT NULL DEFAULT 'unbound' CHECK(binding_phase IN ('unbound','active','transferring')),
 ADD transfer_id text NOT NULL DEFAULT '', ADD target_replica_id text NOT NULL DEFAULT '',
 ADD target_incarnation_id text NOT NULL DEFAULT '', ADD execution_epoch bigint NOT NULL DEFAULT 1,
 ADD expected_execution_epoch bigint NOT NULL DEFAULT 1,
 ADD token_hash text NOT NULL DEFAULT '', ADD token_key_id text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX agent_session_token ON media_agent_gateway.agent_sessions(token_hash) WHERE token_hash<>'';
CREATE INDEX session_binding_owner ON media_agent_gateway.agent_sessions(workspace_id,environment_id,replica_id,incarnation_id);
CREATE INDEX session_transfer_pending ON media_agent_gateway.agent_sessions(updated_at) WHERE binding_phase='transferring';
