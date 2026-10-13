CREATE TABLE agent_workload_targets (
 workspace_id text PRIMARY KEY,agent_config_id text NOT NULL,environment_id text NOT NULL,
 owner_user_id text NOT NULL,spec jsonb NOT NULL,replicas integer NOT NULL CHECK(replicas BETWEEN 0 AND 64),
 desired_replicas integer NOT NULL CHECK(desired_replicas BETWEEN 0 AND 64),workspace_version bigint NOT NULL CHECK(workspace_version>0),
 version bigint NOT NULL CHECK(version>0)
);
CREATE TABLE agent_workload_requests (
 workspace_id text NOT NULL,idempotency_key text NOT NULL,request_hash text NOT NULL,target jsonb NOT NULL,
 PRIMARY KEY(workspace_id,idempotency_key)
);
