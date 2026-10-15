DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_agent_gateway.mcp_definitions WHERE organization_id <> '' OR authentication_type IN ('bearer','api_key')) OR EXISTS(SELECT 1 FROM media_agent_gateway.mcp_definition_versions WHERE organization_id <> '' OR authentication_type IN ('bearer','api_key')) THEN
 RAISE EXCEPTION 'enterprise MCP definitions must be exported or removed before rollback';
 END IF;
END $$;
ALTER TABLE media_agent_gateway.mcp_definitions DROP CONSTRAINT enterprise_mcp_transport;
ALTER TABLE media_agent_gateway.mcp_definition_versions DROP CONSTRAINT enterprise_mcp_transport;
ALTER TABLE media_agent_gateway.mcp_definitions DROP CONSTRAINT mcp_definitions_authentication_type_check;
ALTER TABLE media_agent_gateway.mcp_definition_versions DROP CONSTRAINT mcp_definition_versions_authentication_type_check;
ALTER TABLE media_agent_gateway.mcp_definitions ADD CONSTRAINT mcp_definitions_authentication_type_check CHECK(authentication_type IN ('session_context','oauth','none'));
ALTER TABLE media_agent_gateway.mcp_definition_versions ADD CONSTRAINT mcp_definitions_authentication_type_check CHECK(authentication_type IN ('session_context','oauth','none'));
DROP INDEX media_agent_gateway.mcp_definitions_organization;
ALTER TABLE media_agent_gateway.mcp_definitions DROP COLUMN organization_id;
ALTER TABLE media_agent_gateway.mcp_definition_versions DROP COLUMN organization_id;
