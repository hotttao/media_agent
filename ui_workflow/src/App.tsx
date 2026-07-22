/**
 * Standalone root. Gate on auth: while loading or unauthenticated, show a
 * spinner and trigger the OAuth redirect (login() jumps to
 * /api/v1/auth/login; the materials/workflow auth middleware does the full
 * PKCE flow and 302s back with the mw_token cookie set). Once authenticated,
 * mount the Desktop shell.
 */
import { useEffect } from "react";
import { Desktop } from "@/components/shell/Desktop";
import { Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/foundation/providers/auth-provider";

export default function App() {
  const { user, isLoading, login } = useAuth();

  useEffect(() => {
    if (!isLoading && !user) {
      login(window.location.pathname);
    }
  }, [isLoading, user, login]);

  if (isLoading || !user) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  return <Desktop />;
}
