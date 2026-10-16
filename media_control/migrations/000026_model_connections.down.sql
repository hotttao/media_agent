ALTER TABLE agent_configurations DROP CONSTRAINT model_credential_exclusive;
CREATE OR REPLACE FUNCTION capture_configuration_version() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND (NEW.version<OLD.version OR (NEW.version=OLD.version AND (NEW.agent_type IS DISTINCT FROM OLD.agent_type OR NEW.encrypted_api_key IS DISTINCT FROM OLD.encrypted_api_key))) THEN RAISE EXCEPTION 'configuration_version_conflict' USING ERRCODE='40001';END IF;
 INSERT INTO agent_configuration_versions(agent_config_id,version,agent_type,encrypted_api_key) VALUES(NEW.id,NEW.version,NEW.agent_type,NEW.encrypted_api_key) ON CONFLICT DO NOTHING;RETURN NEW;END $$;
ALTER TABLE agent_configuration_versions DROP COLUMN model_connection_id;
ALTER TABLE agent_configurations DROP COLUMN model_connection_id;
DROP TABLE model_connections;
