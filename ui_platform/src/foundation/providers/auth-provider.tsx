/**
 * Auth provider - manages user session state.
 *
 * Cookie-based: the API client sends `credentials: 'include'`.
 * On mount, calls `api.getMe()` to restore session from cookie.
 *
 * Adapted from agent-platform/web/src/contexts/AuthContext.tsx.
 */

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { api } from "@/foundation/lib/api-client";
import type { User } from "@/foundation/types/auth";

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<void>;
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

  const login = useCallback(async (username: string, password: string) => {
    const user = await api.login(username, password);
    setUser(user);
  }, []);

  const logout = useCallback(async () => {
    const logoutURL = await api.createLogoutURL();
    // Kratos owns completion: navigating to its one-time URL invalidates the
    // httpOnly session cookie and redirects back to /login.
    window.location.assign(logoutURL);
  }, []);

  const refreshUser = useCallback(async () => {
    try {
      const u = await api.getMe();
      setUser(u);
    } catch {
      /* ignore */
    }
  }, []);

  return (
    <AuthContext.Provider
      value={{ user, isLoading, login, logout, refreshUser }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}
