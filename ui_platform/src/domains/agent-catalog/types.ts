export type CatalogKind = "prompts" | "skills" | "mcp";
export interface Grant {
  team_id: string;
  permission: "viewer" | "editor";
}
export interface CatalogResource {
  id: string;
  name: string;
  organization_id: string;
  owner_user_id: string;
  visibility: "private" | "team" | "public";
  my_permission: "owner" | "editor" | "viewer";
  grants?: Grant[];
  version: number;
  content?: string;
  status?: string;
  description?: string;
  published_version?: number;
  has_draft?: boolean;
  source_kind?: string;
  source?: {
    url: string;
    branch: string;
    subpath: string;
    commit: string;
    credential_configured?: boolean;
  };
}
export interface ContentVersion {
  version: number;
  name?: string;
  content?: string;
  slug?: string;
  description?: string;
  created_at?: number;
}
export interface SkillFile {
  path: string;
  size?: number;
  binary?: boolean;
  executable?: boolean;
  content?: string;
  expected_version?: number;
}
export interface MCPDefinition {
  definition_id: string;
  organization_id?: string;
  service_name: string;
  endpoint: string;
  transport: "platform" | "streamable_http" | "sse";
  authentication_type:
    | "none"
    | "bearer"
    | "api_key"
    | "oauth"
    | "session_context";
  status: "enabled" | "disabled" | "retired";
  version: number;
}
export interface MCPBinding {
  binding_id: string;
  workspace_id: string;
  definition_id: string;
  definition_version: number;
  version: number;
  enabled: boolean;
  credential_header?: string;
  credential_configured?: boolean;
  connection_status?: string;
  oauth_authorize_url?: string;
  allowed_methods?: string[];
  allowed_tools?: string[];
  allowed_prompts?: string[];
  allowed_resources?: string[];
}
