import { AppHeaderButton } from "@/components/shell/windows/AppHeaderButton";
import { useAppHeaderSlot } from "@/foundation/providers/app-header-slot";
import { useAuth } from "@/foundation/providers/auth-provider";
import {
  useInstancePersistentState,
  useInstanceState,
} from "@/foundation/stores/instance-state-store";
import type { HostShim } from "./types";
import type { ShellContext } from "./types";

const standaloneContext: ShellContext = {
  organization: { id: "local-org", name: "Local Organization" },
  team: { id: "default-team", name: "Default Team" },
  workspace: { id: "standalone", name: "Standalone Workspace" },
  capabilities: ["materials.read", "materials.write", "workflow.read", "workflow.write"],
};

/**
 * Standalone implementation: uses the local AppWindow's header slot, the
 * foundation instance-state-store, and the local AuthProvider (OAuth via
 * materials/workflow auth middleware).
 */
export const standaloneShim: HostShim = {
  useAppHeaderSlot,
  AppHeaderButton,
  useInstancePersistentState,
  useInstanceState,
  useCurrentUser: useAuth,
  useShellContext: () => standaloneContext,
};
