ALTER TABLE media_agent_gateway.agent_sessions
    ADD COLUMN chat_status text NOT NULL DEFAULT 'human';

ALTER TABLE media_agent_gateway.agent_sessions
    ADD CONSTRAINT agent_sessions_chat_status_check
    CHECK (chat_status IN ('human', 'agent'));

UPDATE media_agent_gateway.agent_sessions AS session
SET chat_status = 'agent'
WHERE EXISTS (
    SELECT 1
    FROM media_agent_gateway.turns AS turn
    WHERE turn.session_id = session.id
      AND turn.status = 'running'
);
