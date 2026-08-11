DROP INDEX IF EXISTS media_agent_gateway.agent_sessions_workspace_updated_idx;
ALTER TABLE media_agent_gateway.agent_sessions
    DROP COLUMN IF EXISTS context_version,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS agent_type;
