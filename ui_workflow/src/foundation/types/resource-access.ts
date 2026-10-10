export interface ResourceGrant {
  team_id: string;
  permission: "viewer" | "editor";
  granted_by?: string;
  granted_at?: string | number;
}

export function allows(
  resource: { allowed_actions?: readonly string[] } | undefined,
  action: string,
): boolean {
  return resource?.allowed_actions?.includes(action) === true;
}
