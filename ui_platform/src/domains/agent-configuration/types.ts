export interface ResourceReference {
  kind: string;
  id: string;
  version: number;
  checksum?: string;
}
export interface PromptSelection {
  mode: "none" | "custom" | "library";
  content?: string;
  resource_id?: string;
  resource_version?: number;
}
export interface MetadataSelection {
  agent_config_id: string;
  version: number;
  prompt?: PromptSelection;
  resources?: ResourceReference[];
  target_context_version?: string;
  configured_prompt?: string;
}
export interface ResourceChoice {
  id: string;
  name: string;
  version: number;
  kind: "prompts" | "skills" | "mcp";
  disabled?: boolean;
}
export interface Binding {
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
export interface DefinitionChoice {
  definition_id: string;
  service_name: string;
  version: number;
  authentication_type: string;
  status: string;
  transport: string;
}
