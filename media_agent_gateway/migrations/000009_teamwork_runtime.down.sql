DROP INDEX IF EXISTS media_agent_gateway.agent_tasks_topology_status_idx;
ALTER TABLE media_agent_gateway.agent_tasks
    DROP CONSTRAINT IF EXISTS agent_tasks_topology_reference_check,
    DROP COLUMN IF EXISTS topology_version,
    DROP COLUMN IF EXISTS topology_id;
DROP TABLE IF EXISTS media_agent_gateway.teamwork_topology_inbox;
DROP TABLE IF EXISTS media_agent_gateway.teamwork_topology_versions;
