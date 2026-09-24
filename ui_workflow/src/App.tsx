/**
 * Standalone root. Gate on auth: while loading or unauthenticated, show a
 * spinner and trigger the Kratos browser login flow. Once authenticated,
 * mount the Desktop shell.
 */
import { lazy, Suspense } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { LanguageSwitcher } from "@/foundation/components/language-switcher";
import { useAuth } from "@/foundation/providers/auth-provider";
import { AuthPage } from "@/pages/auth/AuthPage";

const Desktop = lazy(() =>
  import("@/components/shell/Desktop").then((module) => ({
    default: module.Desktop,
  })),
);

function AppLoading() {
  return (
    <div className="flex h-screen items-center justify-center">
      <Spinner className="h-6 w-6" />
    </div>
  );
}

export default function App() {
  const { t } = useTranslation();
  const { user, isLoading, login } = useAuth();
  const pathname = window.location.pathname;

  if (pathname === "/login" || pathname.startsWith("/login/")) {
    return <AuthPage kind="login" />;
  }

  if (pathname === "/registration" || pathname.startsWith("/registration/")) {
    return <AuthPage kind="registration" />;
  }

  if (isLoading) {
    return <AppLoading />;
  }

  if (!user) {
    return (
      <div className="flex h-screen items-center justify-center bg-background text-foreground">
        <div className="flex w-full max-w-sm flex-col gap-4 rounded-lg border p-6 shadow-sm">
          <div>
            <h1 className="text-lg font-semibold">Media Workflow</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("auth.signInHint")}
            </p>
          </div>
          <Button onClick={() => login(window.location.pathname)}>
            {t("auth.signIn")}
          </Button>
          <LanguageSwitcher />
        </div>
      </div>
    );
  }

  return (
    <Suspense fallback={<AppLoading />}>
      <Desktop />
    </Suspense>
  );
}
