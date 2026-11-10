CREATE TABLE media_auth.application_mcp_targets (definition_id text PRIMARY KEY, application_id text NOT NULL REFERENCES media_auth.applications(id), origin text NOT NULL);
