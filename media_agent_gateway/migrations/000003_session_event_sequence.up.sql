ALTER TABLE media_agent_gateway.universal_events
    DROP CONSTRAINT IF EXISTS universal_events_turn_id_sequence_key;

WITH ranked AS (
    SELECT id, row_number() OVER (PARTITION BY session_id ORDER BY created_at, id) AS next_sequence
    FROM media_agent_gateway.universal_events
)
UPDATE media_agent_gateway.universal_events event
SET sequence = ranked.next_sequence
FROM ranked
WHERE event.id = ranked.id;

CREATE UNIQUE INDEX IF NOT EXISTS universal_events_session_sequence_idx
    ON media_agent_gateway.universal_events (session_id, sequence);
