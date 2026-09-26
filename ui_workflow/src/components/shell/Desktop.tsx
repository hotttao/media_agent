/**
 * Standalone desktop shell - a minimal version of agent-platform's
 * components/shell/Desktop.tsx. Mounts a SlotProvider over a fixed 1-column
 * layout (no layout switching / dock / popout layer / fleet scope - those are
 * host features; standalone only needs a single slot to host the materials app).
 *
 * Plugin mode does NOT use this - the host's Desktop provides the shell.
 */

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Boxes, GitBranch, LogOut, Sparkles } from "lucide-react";
import {
  ONE_COLUMN_SLOTS,
  OneColumnLayout,
} from "@/components/shell/layout/OneColumnLayout";
import { useStandaloneApps } from "@/components/shell/useStandaloneApps";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { LanguageSwitcher } from "@/foundation/components/language-switcher";
import { STANDALONE_WORKSPACE_ID } from "@/foundation/lib/constants";
import { hostShim } from "@/foundation/lib/host-shim";
import {
  SlotProvider,
  useRequiredSlotContext,
} from "@/foundation/providers/slot-provider";
import {
  useWorkspaceProfileLoaded,
  useWorkspaceProfileStore,
} from "@/foundation/stores/workspace-profile-store";

export function Desktop() {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const apps = useStandaloneApps();
  const ensureLoaded = useWorkspaceProfileStore((s) => s.ensureLoaded);
  const loaded = useWorkspaceProfileLoaded(STANDALONE_WORKSPACE_ID);

  useEffect(() => {
    ensureLoaded(STANDALONE_WORKSPACE_ID);
  }, [ensureLoaded]);

  if (!loaded || shell.isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  if (shell.error) {
    return (
      <div className="flex h-screen items-center justify-center p-6 text-sm text-destructive">
        {t("shell.companyError")}: {shell.error}
      </div>
    );
  }

  return (
    <SlotProvider
      workspaceId={STANDALONE_WORKSPACE_ID}
      slots={ONE_COLUMN_SLOTS}
      apps={apps}
      fallbackSlotId="slot-a"
    >
      <div className="flex h-screen flex-col bg-transparent text-foreground">
        <ShellHeader shell={shell} />
        <div className="min-h-0 flex-1">
          <OneColumnLayout />
        </div>
      </div>
    </SlotProvider>
  );
}

function ShellHeader({
  shell,
}: {
  shell: ReturnType<typeof hostShim.useShellContext>;
}) {
  const { t } = useTranslation();
  const slots = useRequiredSlotContext();
  const { user, logout } = hostShim.useCurrentUser();
  const active = slots.getState("slot-a").active?.appId;
  const openApp = (appId: string) => {
    const target = slots.ensureInstance(appId);
    slots.activate(target.slotId, target.instanceId);
  };
  return (
    <header className="relative z-20 border-b border-foreground/[0.06] bg-background/85 px-4 py-3 text-sm font-medium shadow-sm backdrop-blur-xl sm:px-6">
      <div className="mx-auto flex max-w-[1800px] flex-wrap items-center justify-between gap-3 sm:gap-5">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex items-center gap-2.5 border-r pr-4">
            <span className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-primary to-violet-500 text-white shadow-lg shadow-primary/20">
              <Sparkles className="h-4 w-4" />
            </span>
            <div className="hidden sm:block">
              <h1 className="font-semibold tracking-tight">Media Agent</h1>
              <p className="text-[10px] font-normal text-muted-foreground">
                Creative workspace
              </p>
            </div>
          </div>
          <nav
            aria-label={t("shell.applications")}
            className="flex items-center gap-1"
          >
            <button
              type="button"
              className={
                active === "materials"
                  ? "inline-flex items-center gap-2 rounded-xl bg-primary/10 px-3 py-2 text-primary"
                  : "inline-flex items-center gap-2 rounded-xl px-3 py-2 text-muted-foreground transition hover:bg-muted hover:text-foreground"
              }
              onClick={() => openApp("materials")}
            >
              <Boxes className="h-4 w-4" />
              {t("nav.materials")}
            </button>
            <button
              type="button"
              className={
                active === "workflow"
                  ? "inline-flex items-center gap-2 rounded-xl bg-primary/10 px-3 py-2 text-primary"
                  : "inline-flex items-center gap-2 rounded-xl px-3 py-2 text-muted-foreground transition hover:bg-muted hover:text-foreground"
              }
              onClick={() => openApp("workflow")}
            >
              <GitBranch className="h-4 w-4" />
              {t("nav.workflow")}
            </button>
          </nav>
        </div>
        <div className="flex items-center gap-3">
          <LanguageSwitcher />
          <span className="hidden text-xs text-muted-foreground lg:inline">
            {shell.organization.name} / {shell.team.name} /{" "}
            {shell.workspace.name}
          </span>
          <span className="hidden rounded-full bg-muted px-3 py-1.5 text-xs text-muted-foreground sm:inline">
            {user?.username}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => void logout()}
          >
            <LogOut className="mr-1.5 h-3.5 w-3.5" />
            {t("shell.signOut")}
          </Button>
        </div>
      </div>
    </header>
  );
}
