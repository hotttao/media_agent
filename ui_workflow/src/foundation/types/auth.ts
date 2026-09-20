/** Auth-related types.
 *
 * User is returned by media_auth after Oathkeeper and Kratos authenticate the
 * browser request.
 */

export interface User {
  id: string;
  username: string;
  role: string;
  auth_source: "password" | "ldap" | "wecom";
  default_prompt_id?: string;
  auto_evolution: boolean;
}
