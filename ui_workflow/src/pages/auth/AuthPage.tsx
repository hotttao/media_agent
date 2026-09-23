import {
  CircleAlert,
  Layers3,
  ShieldCheck,
  Sparkles,
  Workflow,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { browserFlowURL } from "@/foundation/lib/ory-browser";
import {
  loadBrowserFlow,
  type OryAuthFlow,
  type OryBrowserFlow,
  OryFlowError,
  safeReturnURL,
  submitBrowserFlow,
} from "@/foundation/lib/ory-flow";
import { OryFlowForm } from "./OryFlowForm";

interface AuthPageProps {
  kind: OryAuthFlow;
}

const copy = {
  login: {
    eyebrow: "Welcome back",
    title: "Sign in to your workspace",
    description:
      "Continue building, reviewing, and managing your media workflows.",
    alternate: "New to Media Agent?",
    alternateAction: "Create an account",
    alternateHref: "/registration",
  },
  registration: {
    eyebrow: "Get started",
    title: "Create your account",
    description: "Set up your profile, then choose a secure sign-in method.",
    alternate: "Already have an account?",
    alternateAction: "Sign in",
    alternateHref: "/login",
  },
} as const;

function startFlow(kind: OryAuthFlow) {
  window.location.replace(browserFlowURL(kind, "/"));
}

export function AuthPage({ kind }: AuthPageProps) {
  const didStartFlow = useRef(false);
  const [flow, setFlow] = useState<OryBrowserFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const content = copy[kind];

  useEffect(() => {
    const flowID = new URLSearchParams(window.location.search).get("flow");
    if (!flowID) {
      if (!didStartFlow.current) {
        didStartFlow.current = true;
        startFlow(kind);
      }
      return;
    }

    let active = true;
    loadBrowserFlow(kind, flowID)
      .then((nextFlow) => {
        if (active) setFlow(nextFlow);
      })
      .catch((cause: unknown) => {
        if (!active) return;
        if (
          cause instanceof OryFlowError &&
          [403, 404, 410].includes(cause.status)
        ) {
          setError("This authentication form has expired. Please try again.");
          return;
        }
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to load the authentication form",
        );
      });
    return () => {
      active = false;
    };
  }, [kind]);

  const handleSubmit = async (values: Record<string, string>) => {
    if (!flow) return;
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await submitBrowserFlow(kind, flow, values);
      if (result.status === "flow") {
        setFlow(result.flow);
        return;
      }
      window.location.assign(safeReturnURL(flow.return_to));
    } catch (cause) {
      if (
        cause instanceof OryFlowError &&
        [403, 404, 410].includes(cause.status)
      ) {
        setError("This authentication form has expired. Please try again.");
        return;
      }
      setError(
        cause instanceof Error
          ? cause.message
          : "Authentication failed. Please try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="relative min-h-screen overflow-hidden bg-background text-foreground">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_15%,oklch(var(--primary)/0.18),transparent_34%),radial-gradient(circle_at_85%_80%,oklch(var(--primary)/0.10),transparent_30%)]" />
      <div className="relative mx-auto grid min-h-screen max-w-7xl items-center gap-12 px-5 py-10 lg:grid-cols-[1.05fr_0.95fr] lg:px-12">
        <section
          className="hidden max-w-xl lg:block"
          aria-label="Product introduction"
        >
          <div className="mb-10 inline-flex items-center gap-3">
            <span className="grid h-11 w-11 place-items-center rounded-xl bg-primary text-primary-foreground shadow-lg shadow-primary/20">
              <Sparkles className="h-5 w-5" />
            </span>
            <div>
              <p className="text-lg font-semibold tracking-tight">
                Media Agent
              </p>
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                Creative operations
              </p>
            </div>
          </div>
          <h2 className="max-w-lg text-4xl font-semibold leading-tight tracking-[-0.035em] xl:text-5xl">
            Turn ideas into repeatable media workflows.
          </h2>
          <p className="mt-5 max-w-lg text-base leading-7 text-muted-foreground">
            Keep materials, generation steps, reviews, and team access in one
            focused workspace.
          </p>
          <div className="mt-10 grid max-w-lg grid-cols-2 gap-3">
            <div className="rounded-xl border bg-card/55 p-4 backdrop-blur-sm">
              <Layers3 className="mb-3 h-5 w-5 text-primary" />
              <p className="text-sm font-medium">Shared materials</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                Organized assets for every team.
              </p>
            </div>
            <div className="rounded-xl border bg-card/55 p-4 backdrop-blur-sm">
              <Workflow className="mb-3 h-5 w-5 text-primary" />
              <p className="text-sm font-medium">Visible workflows</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                Clear steps, reviews, and results.
              </p>
            </div>
          </div>
        </section>

        <section className="mx-auto w-full max-w-md">
          <div className="mb-7 flex items-center gap-3 lg:hidden">
            <span className="grid h-10 w-10 place-items-center rounded-xl bg-primary text-primary-foreground">
              <Sparkles className="h-5 w-5" />
            </span>
            <span className="font-semibold">Media Agent</span>
          </div>
          <Card className="border-border/80 bg-card/90 shadow-2xl shadow-black/10 backdrop-blur-xl">
            <CardHeader className="space-y-3 px-6 pb-5 pt-7 sm:px-8 sm:pt-8">
              <div className="flex items-center justify-between">
                <p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">
                  {content.eyebrow}
                </p>
                <ShieldCheck
                  className="h-5 w-5 text-muted-foreground"
                  aria-label="Secure authentication"
                />
              </div>
              <h1 className="text-2xl font-semibold leading-none tracking-tight">
                {content.title}
              </h1>
              <CardDescription className="leading-6">
                {content.description}
              </CardDescription>
            </CardHeader>
            <CardContent className="px-6 pb-7 sm:px-8 sm:pb-8">
              {!flow && !error && (
                <div
                  className="flex min-h-44 items-center justify-center"
                  role="status"
                  aria-label="Loading authentication form"
                >
                  <Spinner className="h-6 w-6" />
                </div>
              )}
              {!flow && error && (
                <div className="space-y-4">
                  <Alert variant="destructive">
                    <CircleAlert className="h-4 w-4" />
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                  <Button className="w-full" onClick={() => startFlow(kind)}>
                    Try again
                  </Button>
                </div>
              )}
              {flow && (
                <>
                  <OryFlowForm
                    flow={flow}
                    isSubmitting={isSubmitting}
                    error={error}
                    onSubmit={handleSubmit}
                  />
                  {kind === "login" && (
                    <div className="mt-4 text-right">
                      <a
                        className="text-sm text-primary hover:underline"
                        href="/recovery"
                      >
                        Forgot your password?
                      </a>
                    </div>
                  )}
                </>
              )}
              <div className="mt-7 border-t pt-6 text-center text-sm text-muted-foreground">
                {content.alternate}{" "}
                <a
                  className="font-medium text-foreground hover:text-primary"
                  href={content.alternateHref}
                >
                  {content.alternateAction}
                </a>
              </div>
            </CardContent>
          </Card>
          <p className="mt-5 text-center text-xs leading-5 text-muted-foreground">
            Authentication is protected by secure, httpOnly sessions.
          </p>
        </section>
      </div>
    </main>
  );
}
