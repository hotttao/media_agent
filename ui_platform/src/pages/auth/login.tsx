/**
 * Login page - Kratos password flow + WeCom SSO (optional).
 *
 * Adapted from agent-platform/web/src/pages/LoginPage.tsx.
 * API: Kratos browser login flow, GET /api/auth/wecom/enabled,
 * GET /api/auth/wecom/authorize
 */

import { MessageSquare } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PasswordInput } from "@/components/ui/password-input";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/foundation/lib/api-client";
import { useAuth } from "@/foundation/providers/auth-provider";

const OAUTH_ERRORS: Record<string, string> = {
  missing_code: "Missing authorization code. Please try again.",
  not_bound:
    "Your WeCom account is not bound to any user. Please login first and bind.",
  user_not_found: "User not found. Please contact admin.",
  system_account: "System accounts cannot login.",
  auth_required: "Please login first to bind your WeCom account.",
  oauth_failed: "WeCom authentication failed. Please try again.",
};

export function LoginPage() {
  const { user, isLoading: authLoading, login } = useAuth();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [wecomEnabled, setWecomEnabled] = useState(false);
  const [wecomLoading, setWecomLoading] = useState(false);

  useEffect(() => {
    const err = searchParams.get("error");
    if (err) setError(OAUTH_ERRORS[err] || "Login failed. Please try again.");
  }, [searchParams]);

  useEffect(() => {
    api
      .getWeComEnabled()
      .then((res) => setWecomEnabled(res.enabled))
      .catch(() => {});
  }, []);

  if (authLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  const nextParam = searchParams.get("next");
  const next =
    nextParam?.startsWith("/") && !nextParam.startsWith("//") ? nextParam : "/";

  if (user) {
    return <Navigate to={next} replace />;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setIsLoading(true);
    try {
      await login(username, password);
      navigate(next);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Login failed. Please try again.",
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleWeComLogin = async () => {
    setWecomLoading(true);
    try {
      const { url } = await api.getWeComAuthorizeUrl("login");
      window.location.href = url;
    } catch {
      setError("Failed to start WeCom login. Please try again.");
      setWecomLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="flex flex-col items-center space-y-3 text-center">
          <div className="space-y-1">
            <CardTitle className="text-2xl font-bold">Media Platform</CardTitle>
            <CardDescription>Sign in to your account</CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">Email</Label>
              <Input
                id="username"
                type="text"
                placeholder="Enter your email"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
                disabled={isLoading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <PasswordInput
                id="password"
                placeholder="Enter your password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                disabled={isLoading}
              />
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? (
                <>
                  <Spinner size="sm" />
                  Signing in...
                </>
              ) : (
                "Sign in"
              )}
            </Button>
          </form>

          {wecomEnabled && (
            <>
              <div className="relative my-4">
                <Separator />
                <span className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-card px-2 text-xs text-muted-foreground">
                  or
                </span>
              </div>
              <Button
                variant="outline"
                className="w-full gap-2"
                onClick={handleWeComLogin}
                disabled={wecomLoading}
              >
                {wecomLoading ? (
                  <Spinner size="sm" />
                ) : (
                  <MessageSquare className="h-4 w-4" />
                )}
                Sign in with WeCom
              </Button>
            </>
          )}

          <div className="mt-4 text-center text-sm text-muted-foreground">
            Don't have an account?{" "}
            <Link
              to="/register"
              className="font-medium text-primary underline-offset-4 hover:underline"
            >
              Register
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
