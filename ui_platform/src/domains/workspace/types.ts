export type AgentType = "codex" | "claude-code";

export const AGENT_TYPE_OPTIONS: ReadonlyArray<{
  value: AgentType;
  label: string;
}> = [
  { value: "codex", label: "Codex" },
  { value: "claude-code", label: "Claude Code" },
];

export function agentTypeLabel(value: AgentType): string {
  return (
    AGENT_TYPE_OPTIONS.find((option) => option.value === value)?.label ?? value
  );
}

export interface Workspace {
  id: string;
  name: string;
  agent_id: string;
  agent_type: AgentType;
  api_key_configured: boolean;
  configuration_version: number;
  created_at: number;
  updated_at: number;
}

export interface CreateWorkspaceInput {
  name: string;
  agent_type: AgentType;
  api_key: string;
}
