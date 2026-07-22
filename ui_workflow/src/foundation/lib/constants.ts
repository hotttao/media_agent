/**
 * Shared constants.
 *
 * Traefik routes /api/v1/{materials,products,virtual-ips,asset-kinds,
 * generated-assets} to media_materials and /api/v1/{workflows,templates,jobs,
 * step-runs} to media_workflow. Both services share the same JWT_SECRET and
 * mw_token cookie domain, so one OAuth login (via /api/v1/auth/* on either
 * service) authenticates both.
 */
export const API_BASE = "/api/v1";

/**
 * Standalone has no workspace concept; use a fixed id for the
 * workspace_profile / instance-state keyspacing (slot default instance ids are
 * deterministic, so a workspace dimension is still needed to isolate state -
 * see agent-platform stores/instance-state-store.ts:26-32). Plugin mode uses
 * the host's real workspaceId from window.tos.workspace.
 */
export const STANDALONE_WORKSPACE_ID = "standalone";

/** localStorage key for i18n language (shared with agent-platform host). */
export const LANGUAGE_STORAGE_KEY = "tos-language";
