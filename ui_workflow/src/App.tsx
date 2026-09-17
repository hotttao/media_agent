/**
 * Standalone root. Gate on auth: while loading or unauthenticated, show a
 * spinner and trigger the OAuth redirect (login() jumps to
 * /api/v1/auth/login; the materials/workflow auth middleware does the full
 * PKCE flow and 302s back with the mw_token cookie set). Once authenticated,
 * mount the Desktop shell.
 */
import { Desktop } from "@/components/shell/Desktop";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/foundation/providers/auth-provider";

export default function App() {
  const { user, isLoading, login } = useAuth();

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  if (!user) {
    return (
      <div className="flex h-screen items-center justify-center bg-background text-foreground">
        <div className="flex w-full max-w-sm flex-col gap-4 rounded-lg border p-6 shadow-sm">
          <div>
            <h1 className="text-lg font-semibold">Media Workflow</h1>
            <p className="mt-1 text-sm text-muted-foreground">Sign in to use Materials and Workflow.</p>
          </div>
          <Button onClick={() => login(window.location.pathname)}>Sign in</Button>
        </div>
      </div>
    );
  }

  return <Desktop />;
}
