import { useEffect, useState } from "react";
import { AppHeaderButton } from "@/components/shell/windows/AppHeaderButton";
import { api } from "@/foundation/lib/api-client";
import { useAppHeaderSlot } from "@/foundation/providers/app-header-slot";
import { useAuth } from "@/foundation/providers/auth-provider";
import { useSlotContext } from "@/foundation/providers/slot-provider";
import { WORKFLOW_LAUNCH_KEY } from "@/foundation/types/app-navigation";
import { requestKey } from "@/foundation/lib/request-key";
import {
  useInstancePersistentState,
  useInstanceState,
  setInstanceState,
} from "@/foundation/stores/instance-state-store";
import type { HostShim, ShellContext } from "./types";

const emptyContext: ShellContext = {
  organization: { id: "", name: "Loading company…" },
  team: { id: "", name: "No department" },
  workspace: { id: "standalone", name: "Standalone Workspace" },
  capabilities: [
    "materials.read",
    "materials.write",
    "workflow.read",
    "workflow.write",
  ],
  isLoading: true,
};

interface OrganizationSummary {
  id: string;
  name: string;
}

interface OrganizationsResponse {
  organizations?: OrganizationSummary[];
}

function useStandaloneShellContext(): ShellContext {
  const [context, setContext] = useState<ShellContext>(emptyContext);

  useEffect(() => {
    let active = true;
    fetch("/api/organizations", { credentials: "include" })
      .then(async (response) => {
        if (!response.ok) throw new Error(`Request failed: ${response.status}`);
        return (await response.json()) as OrganizationsResponse;
      })
      .then(({ organizations = [] }) => {
        if (!active) return;
        const organization = organizations[0];
        if (!organization)
          throw new Error("No company is available for this account");
        api.setOrganizationId(organization.id);
        setContext({
          ...emptyContext,
          organization,
          isLoading: false,
        });
      })
      .catch((error: unknown) => {
        if (!active) return;
        setContext({
          ...emptyContext,
          isLoading: false,
          error:
            error instanceof Error ? error.message : "Unable to load company",
        });
      });
    return () => {
      active = false;
    };
  }, []);

  return context;
}

/**
 * Standalone implementation: uses the local AppWindow's header slot, the
 * foundation instance-state-store, and the local AuthProvider (OAuth via
 * materials/workflow auth middleware).
 */
export const standaloneShim: HostShim = {
  useNavigationPort: () => {
    const slots = useSlotContext();
    return slots
      ? {
          openWorkflowRun(request) {
            const target = slots.ensureInstance("workflow");
            setInstanceState(
              slots.workspaceId,
              target.instanceId,
              WORKFLOW_LAUNCH_KEY,
              { ...request, kind: "workflow_run", requestId: requestKey() },
            );
            slots.activate(target.slotId, target.instanceId);
          },
        }
      : undefined;
  },
  useAppHeaderSlot,
  AppHeaderButton,
  useInstancePersistentState,
  useInstanceState,
  useCurrentUser: useAuth,
  useShellContext: useStandaloneShellContext,
};
