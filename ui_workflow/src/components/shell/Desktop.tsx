/**
 * Standalone desktop shell - a minimal version of agent-platform's
 * components/shell/Desktop.tsx. Mounts a SlotProvider over a fixed 1-column
 * layout (no layout switching / dock / popout layer / fleet scope - those are
 * host features; standalone only needs a single slot to host the materials app).
 *
 * Plugin mode does NOT use this - the host's Desktop provides the shell.
 */
import { ONE_COLUMN_SLOTS, OneColumnLayout } from "@/components/shell/layout/OneColumnLayout";
import { useStandaloneApps } from "@/components/shell/useStandaloneApps";
import { Spinner } from "@/components/ui/spinner";
import { STANDALONE_WORKSPACE_ID } from "@/foundation/lib/constants";
import { SlotProvider } from "@/foundation/providers/slot-provider";
import {
  useWorkspaceProfileLoaded,
  useWorkspaceProfileStore,
} from "@/foundation/stores/workspace-profile-store";
import { useEffect } from "react";

export function Desktop() {
  const apps = useStandaloneApps();
  const ensureLoaded = useWorkspaceProfileStore((s) => s.ensureLoaded);
  const loaded = useWorkspaceProfileLoaded(STANDALONE_WORKSPACE_ID);

  useEffect(() => {
    ensureLoaded(STANDALONE_WORKSPACE_ID);
  }, [ensureLoaded]);

  if (!loaded) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner className="h-6 w-6" />
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
        <header className="border-b border-foreground/[0.06] px-4 py-2 text-sm font-medium">
          media_workflow
        </header>
        <div className="min-h-0 flex-1">
          <OneColumnLayout />
        </div>
      </div>
    </SlotProvider>
  );
}
