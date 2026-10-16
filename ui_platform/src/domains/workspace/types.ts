export type AgentType = "codex" | "claude-code" | "goose";

export const AGENT_TYPE_OPTIONS: ReadonlyArray<{
  value: AgentType;
  label: string;
}> = [
  { value: "codex", label: "Codex" },
  { value: "goose", label: "Goose" },
  { value: "claude-code", label: "Claude Code" },
];

export function agentTypeLabel(value: AgentType): string {
  return (
    AGENT_TYPE_OPTIONS.find((option) => option.value === value)?.label ?? value
  );
}

export interface Workspace {
  organization_id?: string;
  team_id?: string;
  agent_configuration_id?: string;
  id: string;
  name: string;
  agent_id: string;
  agent_type: AgentType;
  api_key_configured: boolean;
  model_connection_id?: string;
  configuration_version: number;
  created_at: number;
  updated_at: number;
}

export interface CreateWorkspaceInput {
  organization_id: string;
  team_id: string;
  name: string;
  agent_type: AgentType;
  api_key: string;
  model_connection_id?: string;
}
