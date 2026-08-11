ALTER TABLE media_agent_gateway.agent_sessions
    ADD COLUMN IF NOT EXISTS agent_type text NOT NULL DEFAULT 'codex',
    ADD COLUMN IF NOT EXISTS title text NOT NULL DEFAULT 'New session',
    ADD COLUMN IF NOT EXISTS context_version bigint NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS agent_sessions_workspace_updated_idx
    ON media_agent_gateway.agent_sessions (user_id, workspace_id, updated_at DESC);
