import type {
  AgentType,
  CreateWorkspaceInput,
  Workspace,
} from "@/domains/workspace/types";
import { appPath } from "@/foundation/lib/app-path";

async function checked(response: Response): Promise<Response> {
  if (response.status === 401) {
    window.location.href = appPath("/login");
    throw new Error("Session expired");
  }
  if (!response.ok) {
    const body = await response
      .json()
      .catch(() => ({ error: "Request failed" }));
    throw new Error(
      typeof body.error === "string" ? body.error : "Request failed",
    );
  }
  return response;
}

export async function listWorkspaces(): Promise<Workspace[]> {
  const response = await checked(
    await fetch("/api/v1/workspaces", { credentials: "include" }),
  );
  const body = (await response.json()) as { workspaces?: Workspace[] };
  return body.workspaces ?? [];
}

export async function createWorkspace(
  input: CreateWorkspaceInput,
): Promise<Workspace> {
  const response = await checked(
    await fetch("/api/v1/workspaces", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  );
  return response.json();
}

export async function updateWorkspace(
  workspaceId: string,
  input: {
    name: string;
    agent_type: AgentType;
    api_key: string;
    model_connection_id?: string;
    model_protocol?: string;
    model_name?: string;
    model_base_url?: string;
  },
): Promise<Workspace> {
  const response = await checked(
    await fetch(`/api/v1/workspaces/${workspaceId}`, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  );
  return response.json();
}

export async function listAvailableModelConnections(): Promise<
  import("@/foundation/types/model-connection").ModelConnectionChoice[]
> {
  const response = await checked(
    await fetch("/api/v1/model-connections", { credentials: "include" }),
  );
  return (
    (
      (await response.json()) as {
        connections?: import("@/foundation/types/model-connection").ModelConnectionChoice[];
      }
    ).connections ?? []
  );
}
