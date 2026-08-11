export interface Workspace {
  id: string;
  name: string;
  agent_id: string;
  agent_type: string;
  api_key_configured: boolean;
  configuration_version: number;
  created_at: number;
  updated_at: number;
}

export interface CreateWorkspaceInput {
  name: string;
  agent_type: "codex";
  api_key: string;
}
