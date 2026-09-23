/**
 * Standalone desktop shell - a minimal version of agent-platform's
 * components/shell/Desktop.tsx. Mounts a SlotProvider over a fixed 1-column
 * layout (no layout switching / dock / popout layer / fleet scope - those are
 * host features; standalone only needs a single slot to host the materials app).
 *
 * Plugin mode does NOT use this - the host's Desktop provides the shell.
 */

import { useEffect } from "react";
import {
  ONE_COLUMN_SLOTS,
  OneColumnLayout,
} from "@/components/shell/layout/OneColumnLayout";
import { useStandaloneApps } from "@/components/shell/useStandaloneApps";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
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
        Unable to load company context: {shell.error}
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
      <div className="flex h-screen flex-col bg-background text-foreground">
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
  const slots = useRequiredSlotContext();
  const { user, logout } = hostShim.useCurrentUser();
  const active = slots.getState("slot-a").active?.appId;
  const openApp = (appId: string) => {
    const target = slots.ensureInstance(appId);
    slots.activate(target.slotId, target.instanceId);
  };
  return (
    <header className="border-b border-foreground/[0.06] px-4 py-2 text-sm font-medium">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <h1 className="font-semibold">Media Workflow</h1>
          <nav aria-label="Applications" className="flex items-center gap-1">
            <button
              type="button"
              className={
                active === "materials"
                  ? "rounded bg-foreground/10 px-2 py-1"
                  : "rounded px-2 py-1 text-muted-foreground hover:bg-foreground/10"
              }
              onClick={() => openApp("materials")}
            >
              Materials
            </button>
            <button
              type="button"
              className={
                active === "workflow"
                  ? "rounded bg-foreground/10 px-2 py-1"
                  : "rounded px-2 py-1 text-muted-foreground hover:bg-foreground/10"
              }
              onClick={() => openApp("workflow")}
            >
              Workflow
            </button>
          </nav>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground">
            {shell.organization.name} / {shell.team.name} /{" "}
            {shell.workspace.name}
          </span>
          <span className="text-xs text-muted-foreground">
            {user?.username}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => void logout()}
          >
            Sign out
          </Button>
        </div>
      </div>
    </header>
  );
}
