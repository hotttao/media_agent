/** Auth-related types.
 *
 * User is the simplified shape returned by media_workflow / media_materials
 * /api/v1/auth/me (flat JSON, id === sub). Simpler than media_auth's ApiUser
 * (no auth_source / default_prompt_id / auto_evolution) - see
 * media_workflow/biz/shared/auth/middleware.go:36-41,466-479.
 */

export interface User {
  id: string;
  sub: string;
  username: string;
  name: string;
  role: string;
}

/** /api/v1/auth/logout response. */
export interface LogoutResponse {
  success: boolean;
}
