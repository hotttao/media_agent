ALTER TABLE media_agent_gateway.agent_sessions
    DROP CONSTRAINT IF EXISTS agent_sessions_chat_status_check;

ALTER TABLE media_agent_gateway.agent_sessions
    DROP COLUMN IF EXISTS chat_status;
