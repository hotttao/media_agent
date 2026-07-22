import type { ComponentType } from "react";
import type { User } from "@/foundation/types/auth";

export interface CurrentUser {
  user: User | null;
  isLoading: boolean;
  /** Standalone: redirect to /api/v1/auth/login (OAuth). Plugin: rarely used
   * (host owns auth), but provided for interface parity. */
  login: (next?: string) => void;
  logout: () => Promise<void>;
}

/**
 * Unified host-surface for app components. Apps depend only on this interface;
 * standalone and plugin provide different implementations (see ./standalone
 * and ./plugin), build-time selected via VITE_TARGET.
 */
export interface HostShim {
  useAppHeaderSlot: () => HTMLDivElement | null;
  AppHeaderButton: ComponentType<Record<string, unknown>>;
  useInstancePersistentState: <T>(
    instanceId: string,
    key: string,
    defaultFn: () => T,
  ) => [T, (next: T | ((prev: T) => T)) => void];
  useInstanceState: <T>(
    instanceId: string,
    key: string,
    defaultFn: () => T,
  ) => [T, (next: T | ((prev: T) => T)) => void];
  useCurrentUser: () => CurrentUser;
}
