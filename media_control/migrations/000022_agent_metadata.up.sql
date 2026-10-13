CREATE TABLE catalog_resource_versions (
 kind text NOT NULL, resource_id text NOT NULL, version bigint NOT NULL CHECK(version > 0),
 owner_user_id text NOT NULL, workspace_id text, payload jsonb NOT NULL,
 PRIMARY KEY(kind,resource_id,version)
);
CREATE FUNCTION capture_catalog_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 snapshot := to_jsonb(NEW) - 'created_at' - 'updated_at';
 IF TG_OP = 'UPDATE' AND NEW.version <= OLD.version AND snapshot <> (to_jsonb(OLD)-'created_at'-'updated_at') THEN
  RAISE EXCEPTION 'catalog version must increase';
 END IF;
 INSERT INTO catalog_resource_versions(kind,resource_id,version,owner_user_id,workspace_id,payload)
 VALUES (TG_ARGV[0],NEW.id,NEW.version,NEW.owner_user_id,NEW.workspace_id,snapshot)
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER prompt_version AFTER INSERT OR UPDATE ON prompts FOR EACH ROW EXECUTE FUNCTION capture_catalog_version('media_control.configured_prompt');
CREATE TRIGGER skill_version AFTER INSERT OR UPDATE ON skills FOR EACH ROW EXECUTE FUNCTION capture_catalog_version('media_control.skill');
CREATE TRIGGER memory_version AFTER INSERT OR UPDATE ON memory_stores FOR EACH ROW EXECUTE FUNCTION capture_catalog_version('media_control.memory_store');
INSERT INTO catalog_resource_versions SELECT 'media_control.configured_prompt',id,version,owner_user_id,workspace_id,to_jsonb(prompts)-'created_at'-'updated_at' FROM prompts;
INSERT INTO catalog_resource_versions SELECT 'media_control.skill',id,version,owner_user_id,workspace_id,to_jsonb(skills)-'created_at'-'updated_at' FROM skills;
INSERT INTO catalog_resource_versions SELECT 'media_control.memory_store',id,version,owner_user_id,workspace_id,to_jsonb(memory_stores)-'created_at'-'updated_at' FROM memory_stores;
CREATE TABLE agent_metadata_releases (
 id text PRIMARY KEY, workspace_id text NOT NULL, agent_config_id text NOT NULL,
 selection_revision bigint NOT NULL CHECK(selection_revision > 0), request_hash text NOT NULL,
 idempotency_key text NOT NULL, creator_id text NOT NULL, manifest jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(agent_config_id,selection_revision), UNIQUE(agent_config_id,idempotency_key)
);
CREATE TABLE metadata_notification_outbox (
 event_id text PRIMARY KEY, release_id text NOT NULL REFERENCES agent_metadata_releases(id),
 routing_key text NOT NULL, payload jsonb NOT NULL, attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(), delivered_at timestamptz,
 lease_until timestamptz, lease_token text
);
CREATE INDEX metadata_outbox_ready ON metadata_notification_outbox(available_at) WHERE delivered_at IS NULL;
CREATE TABLE metadata_application_reports (
 report_id text PRIMARY KEY, workspace_id text NOT NULL, agent_config_id text NOT NULL,
 selection_revision bigint NOT NULL, target_context_version text NOT NULL,
 replica_id text NOT NULL, incarnation_id text NOT NULL, status text NOT NULL,
 applied_context_version text NOT NULL DEFAULT '', error_code text NOT NULL DEFAULT '',
 content_hash text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status IN ('applied','failed','incompatible','superseded'))
);
CREATE INDEX metadata_report_partition ON metadata_application_reports(agent_config_id,selection_revision,replica_id,incarnation_id);
CREATE TABLE agent_configuration_versions (
 agent_config_id text NOT NULL, version bigint NOT NULL, agent_type text NOT NULL,
 encrypted_api_key bytea, PRIMARY KEY(agent_config_id,version)
);
INSERT INTO agent_configuration_versions SELECT id,version,agent_type,encrypted_api_key FROM agent_configurations;
CREATE FUNCTION capture_configuration_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND (NEW.version<OLD.version OR (NEW.version=OLD.version AND (NEW.agent_type IS DISTINCT FROM OLD.agent_type OR NEW.encrypted_api_key IS DISTINCT FROM OLD.encrypted_api_key))) THEN
  RAISE EXCEPTION 'configuration_version_conflict' USING ERRCODE='40001';
 END IF;
 INSERT INTO agent_configuration_versions VALUES(NEW.id,NEW.version,NEW.agent_type,NEW.encrypted_api_key) ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER configuration_version AFTER INSERT OR UPDATE ON agent_configurations FOR EACH ROW EXECUTE FUNCTION capture_configuration_version();
