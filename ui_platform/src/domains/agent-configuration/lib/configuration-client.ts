import type {
  Binding,
  MetadataSelection,
} from "@/domains/agent-configuration/types";
import {
  ManagementError,
  managementRequest,
} from "@/foundation/lib/management-http";
export function bindingsPath(workspace: string, id = ""): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspace)}/mcp-bindings${id ? `/${encodeURIComponent(id)}` : ""}`;
}
export async function listBindings(workspace: string): Promise<Binding[]> {
  const data = await managementRequest<{ bindings?: Binding[] }>(
    bindingsPath(workspace),
  );
  return data.bindings ?? [];
}
export async function loadSelection(
  workspace: string,
  config: string,
  version: number,
): Promise<MetadataSelection> {
  try {
    return await managementRequest<MetadataSelection>(
      `/api/v1/workspaces/${encodeURIComponent(workspace)}/agent-metadata`,
    );
  } catch (error) {
    if (error instanceof ManagementError && error.status === 404)
      return {
        agent_config_id: config,
        version,
        prompt: { mode: "none" },
        resources: [],
      };
    throw error;
  }
}
