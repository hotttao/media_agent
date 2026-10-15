ALTER TABLE media_agent_gateway.mcp_definitions ADD COLUMN organization_id text NOT NULL DEFAULT '';
ALTER TABLE media_agent_gateway.mcp_definition_versions ADD COLUMN organization_id text NOT NULL DEFAULT '';
CREATE INDEX mcp_definitions_organization ON media_agent_gateway.mcp_definitions(organization_id,id);
ALTER TABLE media_agent_gateway.mcp_definitions DROP CONSTRAINT mcp_definitions_authentication_type_check;
ALTER TABLE media_agent_gateway.mcp_definition_versions DROP CONSTRAINT mcp_definitions_authentication_type_check;
ALTER TABLE media_agent_gateway.mcp_definitions ADD CONSTRAINT mcp_definitions_authentication_type_check CHECK(authentication_type IN ('session_context','oauth','none','bearer','api_key'));
ALTER TABLE media_agent_gateway.mcp_definition_versions ADD CONSTRAINT mcp_definition_versions_authentication_type_check CHECK(authentication_type IN ('session_context','oauth','none','bearer','api_key'));
ALTER TABLE media_agent_gateway.mcp_definitions ADD CONSTRAINT enterprise_mcp_transport CHECK(organization_id = '' OR (transport <> 'platform' AND authentication_type <> 'session_context'));
ALTER TABLE media_agent_gateway.mcp_definition_versions ADD CONSTRAINT enterprise_mcp_transport CHECK(organization_id = '' OR (transport <> 'platform' AND authentication_type <> 'session_context'));
