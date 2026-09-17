/**
 * Auth provider (standalone mode only).
 *
 * On mount, calls /api/v1/auth/me to restore the session from the mw_token
 * cookie. `login()` redirects to /api/v1/auth/login - the materials/workflow
 * auth middleware handles the full OAuth flow (PKCE -> media_auth authorize ->
 * callback -> sets mw_token cookie -> 302 back to `next`). The frontend never
 * touches OAuth code/token.
 *
 * Plugin mode does NOT mount this - it reads the current user via host-shim
 * (the host already authenticated the user).
 *
 * Adapted from ui_platform/src/foundation/providers/auth-provider.tsx (login
 * changed from username/password POST to OAuth redirect).
 */
import { api } from "@/foundation/lib/api-client";
import type { User } from "@/foundation/types/auth";
import { type ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  /** Redirect to /api/v1/auth/login (OAuth via media_auth). */
  login: (next?: string) => void;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    api
      .getMe()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setIsLoading(false));
  }, []);

  const login = useCallback((next?: string) => {
    const q = next ? `?next=${encodeURIComponent(next)}` : "";
    window.location.href = `/api/auth/login${q}`;
  }, []);

  const logout = useCallback(async () => {
    setUser(null);
    window.location.href = "/self-service/logout/browser";
  }, []);

  const refreshUser = useCallback(async () => {
    try {
      setUser(await api.getMe());
    } catch {
      /* ignore */
    }
  }, []);

  return (
    <AuthContext.Provider value={{ user, isLoading, login, logout, refreshUser }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return ctx;
}
