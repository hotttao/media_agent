DROP INDEX IF EXISTS media_agent_gateway.universal_events_session_sequence_idx;

ALTER TABLE media_agent_gateway.universal_events
    ADD CONSTRAINT universal_events_turn_id_sequence_key UNIQUE (turn_id, sequence);
