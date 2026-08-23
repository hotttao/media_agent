import type { ChatSession } from "@/domains/chat/types";

interface SessionConfiguration {
  agent_type: string;
  configuration_version: number;
}

export function isSessionCompatible(
  session: ChatSession | undefined,
  workspace: SessionConfiguration,
): boolean {
  return (
    session !== undefined &&
    session.agent_type === workspace.agent_type &&
    session.context_version === workspace.configuration_version
  );
}
