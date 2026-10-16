CREATE TABLE model_connections (id text PRIMARY KEY, owner_user_id text NOT NULL, name text NOT NULL,kind text NOT NULL CHECK(kind='codex_auth_json'),status text NOT NULL CHECK(status IN ('active','revoked')),version bigint NOT NULL CHECK(version>0),encrypted_auth bytea,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL);
CREATE INDEX model_connections_owner ON model_connections(owner_user_id);
ALTER TABLE agent_configurations ADD COLUMN model_connection_id text NOT NULL DEFAULT '';
ALTER TABLE agent_configuration_versions ADD COLUMN model_connection_id text NOT NULL DEFAULT '';
ALTER TABLE agent_configurations ADD CONSTRAINT model_credential_exclusive CHECK(model_connection_id='' OR encrypted_api_key IS NULL);
CREATE OR REPLACE FUNCTION capture_configuration_version() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND (NEW.version<OLD.version OR (NEW.version=OLD.version AND (NEW.agent_type IS DISTINCT FROM OLD.agent_type OR NEW.encrypted_api_key IS DISTINCT FROM OLD.encrypted_api_key OR NEW.model_connection_id IS DISTINCT FROM OLD.model_connection_id))) THEN RAISE EXCEPTION 'configuration_version_conflict' USING ERRCODE='40001';END IF;
 INSERT INTO agent_configuration_versions(agent_config_id,version,agent_type,encrypted_api_key,model_connection_id) VALUES(NEW.id,NEW.version,NEW.agent_type,NEW.encrypted_api_key,NEW.model_connection_id) ON CONFLICT DO NOTHING;RETURN NEW;END $$;
