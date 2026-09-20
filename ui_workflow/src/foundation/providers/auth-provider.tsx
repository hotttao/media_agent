/**
 * Auth provider (standalone mode only).
 *
 * On mount, calls /api/auth/me to restore the business User from the Kratos
 * browser session. Login and logout use Kratos browser flows; the frontend
 * never reads or stores the httpOnly session cookie.
 *
 * Plugin mode does NOT mount this - it reads the current user via host-shim
 * (the host already authenticated the user).
 *
 * Adapted from ui_platform/src/foundation/providers/auth-provider.tsx (login
 * changed from username/password POST to OAuth redirect).
 */
import { api } from "@/foundation/lib/api-client";
import {
  browserFlowURL,
  createBrowserLogoutURL,
} from "@/foundation/lib/ory-browser";
import type { User } from "@/foundation/types/auth";
import { type ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  /** Redirect to the Kratos browser login flow. */
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
    window.location.assign(browserFlowURL("login", next ?? "/"));
  }, []);

  const logout = useCallback(async () => {
    const logoutURL = await createBrowserLogoutURL("/login");
    setUser(null);
    window.location.assign(logoutURL);
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
