/**
 * media_materials VirtualIp domain types.
 *
 * Field shapes from idl/media_materials/virtual_ip.proto (VirtualIp message).
 * Metadata/Spec/Status pattern is not forced: flat rows, like Material.
 * created_at/updated_at are int64 (UnixNano) in proto; typed as string to
 * match the existing Material types (confirm against handler JSON during
 * integration).
 *
 * ACL: visibility (private/team/public) + grants (virtual_ip_grants table).
 * grants are NOT populated by the current toView (handler returns VirtualIp
 * without grants) - the optional field is kept for forward compatibility;
 * callers track granted team_ids locally for revoke UI.
 */

export type Visibility = "private" | "team" | "public";
export type VirtualIpStatus = "active" | "archived";
export type VirtualIpPermission = "viewer" | "editor";

export interface PersonaProfile {
  age?: number;
  gender?: "MALE" | "FEMALE" | "OTHER";
  height?: number;
  weight?: number;
  bust?: number;
  waist?: number;
  hip?: number;
  education?: string;
  major?: string;
  city?: string;
  occupation?: string;
  income_level?: string;
  basic_setting?: string;
  family_background?: string;
  personality?: string;
  catchphrase?: string;
  small_habit?: string;
  hobbies?: string;
}

// ── VirtualIp (AI 形象 / virtual model) ────────────────────────────────

export interface VirtualIpGrant {
  team_id: string;
  permission: VirtualIpPermission;
  granted_by: string;
  granted_at: string;
}

export interface VirtualIp {
  id: string;
  organization_id?: string;
  scope: string;
  name: string;
  description?: string;
  avatar_object_path?: string;
  full_body_object_path?: string;
  three_view_object_path?: string;
  nine_view_object_path?: string;
  persona_profile?: PersonaProfile | string;
  status: VirtualIpStatus;
  visibility: Visibility;
  owner_user_id: string;
  grants?: VirtualIpGrant[];
  created_at: string | number;
  updated_at: string | number;
}

export interface VirtualIpInput {
  scope: string;
  name: string;
  description?: string;
  avatar_object_path?: string;
  full_body_object_path?: string;
  three_view_object_path?: string;
  nine_view_object_path?: string;
  persona_profile?: PersonaProfile;
  visibility?: Visibility;
}

export interface VirtualIpListParams {
  scope?: string;
  name?: string;
  status?: VirtualIpStatus;
  visibility?: Visibility;
  page?: number;
  page_size?: number;
}

/** Paged response shape - ListVirtualIpsResp { items, total } in proto. */
export interface VirtualIpsPage {
  items: VirtualIp[];
  total: number;
}

// ── VirtualIp ACL inputs ──────────────────────────────────────────────

export interface GrantInput {
  team_id: string;
}

export interface ChangeVisibilityInput {
  visibility: Visibility;
}
