/**
 * OAuth authorize page - consent screen for OAuth2 authorization code flow.
 *
 * User sees the requesting client's name + scope, then approves or denies.
 * On approve: POST /api/oauth/authorize -> redirect to client's redirect_uri.
 * On deny: redirect to client's redirect_uri?error=access_denied.
 *
 * Adapted from agent-platform/web/src/pages/OAuthAuthorizePage.tsx.
 */

import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/foundation/lib/api-client";

export function OAuthAuthorizePage() {
  const [searchParams] = useSearchParams();
  const [loading, setLoading] = useState(true);
  const [clientName, setClientName] = useState("");
  const [scope, setScope] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const clientId = searchParams.get("client_id") || "";
  const redirectUri = searchParams.get("redirect_uri") || "";
  const responseType = searchParams.get("response_type") || "";
  const state = searchParams.get("state") || "";
  const codeChallenge = searchParams.get("code_challenge") || "";
  const codeChallengeMethod = searchParams.get("code_challenge_method") || "";
  const scopeParam = searchParams.get("scope") || "";

  useEffect(() => {
    if (!clientId || !redirectUri) {
      setError("Missing client_id or redirect_uri.");
      setLoading(false);
      return;
    }
    if (responseType !== "code") {
      setError("Unsupported response_type. Only 'code' is supported.");
      setLoading(false);
      return;
    }

    api
      .getOAuthAuthorizeInfo({
        client_id: clientId,
        redirect_uri: redirectUri,
        response_type: responseType,
        scope: scopeParam,
      })
      .then((info) => {
        setClientName(info.client_name);
        setScope(info.scope || "profile");
        setError("");
      })
      .catch(() => {
        setError("Failed to load OAuth client information.");
      })
      .finally(() => setLoading(false));
  }, [clientId, redirectUri, responseType, scopeParam]);

  const handleApprove = async () => {
    setSubmitting(true);
    try {
      const res = await api.approveOAuthAuthorization({
        client_id: clientId,
        redirect_uri: redirectUri,
        response_type: responseType,
        ...(state ? { state } : {}),
        ...(codeChallenge ? { code_challenge: codeChallenge } : {}),
        ...(codeChallengeMethod
          ? { code_challenge_method: codeChallengeMethod }
          : {}),
        ...(scopeParam ? { scope: scopeParam } : {}),
      });
      window.location.href = res.redirect_uri;
    } catch {
      setError("Authorization failed. Please try again.");
      setSubmitting(false);
    }
  };

  const handleDeny = () => {
    const url = new URL(redirectUri);
    url.searchParams.set("error", "access_denied");
    if (state) url.searchParams.set("state", state);
    window.location.href = url.toString();
  };

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Authorize Application</CardTitle>
          <CardDescription>
            {clientName
              ? `"${clientName}" is requesting access to your account.`
              : "An application is requesting access."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {!error && (
            <>
              <div className="rounded-lg border p-4 space-y-2">
                <div className="text-sm">
                  <span className="font-medium">Application:</span> {clientName}
                </div>
                <div className="text-sm">
                  <span className="font-medium">Scope:</span> {scope}
                </div>
                <div className="text-sm">
                  <span className="font-medium">Redirect URI:</span>{" "}
                  <code className="text-xs bg-muted px-1 py-0.5 rounded">
                    {redirectUri}
                  </code>
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  onClick={handleApprove}
                  className="flex-1"
                  disabled={submitting}
                >
                  {submitting ? <Spinner size="sm" /> : "Authorize"}
                </Button>
                <Button
                  variant="outline"
                  onClick={handleDeny}
                  className="flex-1"
                  disabled={submitting}
                >
                  Deny
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
