CREATE SCHEMA IF NOT EXISTS media_plugin;
CREATE TABLE media_plugin.workspace_plugin_bindings (
 id text PRIMARY KEY, workspace_id text NOT NULL, definition_id text NOT NULL,
 version bigint NOT NULL CHECK(version>0), payload jsonb NOT NULL,
 UNIQUE(workspace_id,definition_id)
);
CREATE INDEX plugin_bindings_workspace ON media_plugin.workspace_plugin_bindings(workspace_id,id);
CREATE TABLE media_plugin.plugin_binding_versions (
 id text NOT NULL, workspace_id text NOT NULL, version bigint NOT NULL CHECK(version>0), payload jsonb NOT NULL,
 PRIMARY KEY(id,version)
);
CREATE TABLE media_plugin.mcp_credentials (
 id text PRIMARY KEY, binding_id text NOT NULL, ciphertext bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
