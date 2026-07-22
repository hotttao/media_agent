import { create } from "zustand";

/**
 * Per-workspace UI profile store (standalone mode).
 *
 * Adapted from agent-platform/web/src/stores/workspace-profile-store.ts.
 * Standalone has no server profile endpoint, so the default backend is
 * localStorage (the host's ProfileBackend would hit the server). Slot state +
 * persisted instance state live here.
 *
 * Top-level payload keys (opaque to the store): layout_id, slots (opened/active
 * per slot), instances (per-instance persistent state). Treat every field as
 * optional; missing -> caller-side default.
 */

export type WorkspaceProfilePayload = Record<string, unknown>;

interface State {
  byWorkspace: Record<string, WorkspaceProfilePayload>;
}

interface Actions {
  ensureLoaded(workspaceId: string): void;
  patch(workspaceId: string, patch: WorkspaceProfilePayload): void;
}

/** Backing strategy for a profile id. Default is localStorage. */
export interface ProfileBackend {
  load: (id: string) => Promise<WorkspaceProfilePayload>;
  save: (id: string, patch: WorkspaceProfilePayload) => Promise<WorkspaceProfilePayload>;
}

const LS_PREFIX = "mw-profile:";

const lsBackend: ProfileBackend = {
  load: async (id) => {
    if (typeof localStorage === "undefined") return {};
    const raw = localStorage.getItem(`${LS_PREFIX}${id}`);
    return raw ? (JSON.parse(raw) as WorkspaceProfilePayload) : {};
  },
  save: async (id, patch) => {
    const key = `${LS_PREFIX}${id}`;
    const raw = typeof localStorage !== "undefined" ? localStorage.getItem(key) : null;
    const cur = raw ? (JSON.parse(raw) as WorkspaceProfilePayload) : {};
    const merged = { ...cur, ...patch };
    if (typeof localStorage !== "undefined") {
      localStorage.setItem(key, JSON.stringify(merged));
    }
    return merged;
  },
};

const backendOverrides = new Map<string, ProfileBackend>();

export function registerProfileBackend(id: string, backend: ProfileBackend): void {
  backendOverrides.set(id, backend);
}

function backendFor(id: string): ProfileBackend {
  return backendOverrides.get(id) ?? lsBackend;
}

const loadStarted = new Set<string>();
const flushTimers = new Map<string, ReturnType<typeof setTimeout>>();
const pending = new Map<string, WorkspaceProfilePayload>();
const FLUSH_DEBOUNCE_MS = 400;

export const useWorkspaceProfileStore = create<State & Actions>()((set) => ({
  byWorkspace: {},

  ensureLoaded(workspaceId) {
    if (loadStarted.has(workspaceId)) return;
    loadStarted.add(workspaceId);
    backendFor(workspaceId)
      .load(workspaceId)
      .then((payload) => {
        set((s) => ({ byWorkspace: { ...s.byWorkspace, [workspaceId]: payload } }));
      })
      .catch((err) => {
        loadStarted.delete(workspaceId); // allow retry
        console.error("[workspace-profile] load failed", workspaceId, err);
      });
  },

  patch(workspaceId, patch) {
    set((s) => ({
      byWorkspace: {
        ...s.byWorkspace,
        [workspaceId]: { ...(s.byWorkspace[workspaceId] ?? {}), ...patch },
      },
    }));

    pending.set(workspaceId, { ...(pending.get(workspaceId) ?? {}), ...patch });

    const existing = flushTimers.get(workspaceId);
    if (existing) clearTimeout(existing);
    flushTimers.set(
      workspaceId,
      setTimeout(() => void flush(workspaceId), FLUSH_DEBOUNCE_MS),
    );
  },
}));

async function flush(workspaceId: string) {
  flushTimers.delete(workspaceId);
  const patch = pending.get(workspaceId);
  if (!patch) return;
  pending.delete(workspaceId);
  try {
    const merged = await backendFor(workspaceId).save(workspaceId, patch);
    useWorkspaceProfileStore.setState((s) => ({
      byWorkspace: { ...s.byWorkspace, [workspaceId]: merged },
    }));
  } catch (err) {
    console.error("[workspace-profile] flush failed", workspaceId, err);
    pending.set(workspaceId, { ...patch, ...(pending.get(workspaceId) ?? {}) });
  }
}

const EMPTY: WorkspaceProfilePayload = {};

export function useWorkspaceProfile(workspaceId: string | undefined): WorkspaceProfilePayload {
  return useWorkspaceProfileStore((s) =>
    workspaceId ? (s.byWorkspace[workspaceId] ?? EMPTY) : EMPTY,
  );
}

/** True once the initial load has resolved (even if payload is empty). */
export function useWorkspaceProfileLoaded(workspaceId: string | undefined): boolean {
  return useWorkspaceProfileStore((s) => (workspaceId ? workspaceId in s.byWorkspace : true));
}
