/** Auth-related types (aligned with media_auth backend API). */

export interface User {
  id: string;
  username: string;
  role: "user" | "admin" | "system";
  auth_source: "password" | "ldap" | "wecom";
  default_prompt_id: string | null;
  auto_evolution: boolean;
}

export interface ServiceTokenRecord {
  id: string;
  name: string;
  created_by: string;
  created_at: string;
  revoked_at?: string;
  is_platform: boolean;
}

export interface UserRecord {
  id: string;
  username: string;
  display_name: string;
  email?: string;
  role: string;
  auth_source: string;
  created_at: string;
  last_login_at?: string;
  status: "active" | "disabled" | "deleted";
  auth_version: number;
  deleted_at?: string;
  deleted_by?: string;
}

export interface UsersPage {
  items: UserRecord[];
  total: number;
  page: number;
  page_size: number;
}

export interface WeComEnabledResponse {
  enabled: boolean;
}

export interface WeComAuthorizeResponse {
  url: string;
}

export interface Identity {
  provider: string;
  external_id: string;
  display_name?: string;
  created_at: string;
}

export interface OAuthAuthorizeInfo {
  client_name: string;
  scope: string;
}

export interface OAuthAuthorizeResponse {
  redirect_uri: string;
}
