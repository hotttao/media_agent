import { managementRequest } from "@/foundation/lib/management-http";
import type { Organization, Team } from "@/foundation/types/hierarchy";
export async function listOrganizations(): Promise<Organization[]> {
  const data = await managementRequest<{ organizations?: Organization[] }>(
    "/api/organizations",
  );
  return (data.organizations ?? []).filter((item) => item.status === "active");
}
export async function listMyTeams(): Promise<Team[]> {
  const data = await managementRequest<{ teams?: Team[] }>("/api/teams");
  return (data.teams ?? []).filter((item) => item.status === "active");
}
