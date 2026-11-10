BEGIN;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM application_agent_bindings) THEN RAISE EXCEPTION 'application bindings must be explicitly retired before downgrade'; END IF; END $$;
DROP TABLE application_agent_bindings;
COMMIT;
