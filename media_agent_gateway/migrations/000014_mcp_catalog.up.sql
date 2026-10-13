CREATE TABLE media_agent_gateway.mcp_definitions (
 id text PRIMARY KEY, service_name text NOT NULL, endpoint text NOT NULL,
 transport text NOT NULL CHECK(transport IN ('platform','streamable_http','sse')),
 authentication_type text NOT NULL CHECK(authentication_type IN ('session_context','oauth','none')),
 status text NOT NULL CHECK(status IN ('enabled','disabled','retired')), version bigint NOT NULL CHECK(version>0),
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE media_agent_gateway.mcp_definition_versions (LIKE media_agent_gateway.mcp_definitions INCLUDING DEFAULTS INCLUDING CONSTRAINTS);
ALTER TABLE media_agent_gateway.mcp_definition_versions ADD PRIMARY KEY(id,version);
CREATE TABLE media_agent_gateway.mcp_catalog_events (
 id text PRIMARY KEY,definition_id text NOT NULL,version bigint NOT NULL,
 created_at timestamptz NOT NULL,delivered_at timestamptz,attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(), UNIQUE(definition_id,version)
);
CREATE INDEX mcp_catalog_events_pending ON media_agent_gateway.mcp_catalog_events(available_at) WHERE delivered_at IS NULL;
INSERT INTO media_agent_gateway.mcp_definitions VALUES('platform','platform','platform://builtin','platform','session_context','enabled',1,now(),now());
INSERT INTO media_agent_gateway.mcp_definition_versions SELECT * FROM media_agent_gateway.mcp_definitions;
CREATE TABLE media_agent_gateway.mcp_catalog_revision(singleton boolean PRIMARY KEY CHECK(singleton),version bigint NOT NULL);
INSERT INTO media_agent_gateway.mcp_catalog_revision VALUES(true,1);
