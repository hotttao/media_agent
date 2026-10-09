import { useQuery } from "@tanstack/react-query";
import { api } from "@/foundation/lib/api-client";
import {
  browserFlowURL,
  createBrowserLogoutURL,
} from "@/foundation/lib/ory-browser";
import { getTos } from "@/foundation/lib/plugin-host";
import type { User } from "@/foundation/types/auth";
import type { HostShim, ShellContext } from "./types";

const pluginFallbackContext: ShellContext = {
  organization: { id: "host-org", name: "Organization" },
  team: { id: "host-team", name: "Team" },
  workspace: { id: "host-workspace", name: "Workspace" },
  capabilities: ["materials.read", "workflow.read"],
};

/**
 * Plugin implementation: forwards nap-specific hooks to window.tos
 * (workspace/windows), and reads the current user via /api/v1/auth/me (the
 * host already authenticated the user, so mw_token cookie is present; query
 * uses the plugin-local QueryClient mounted inside each panel per §4.3 方案 C).
 */

function useCurrentUser() {
  const { data, isLoading } = useQuery<User>({
    queryKey: ["auth-me"],
    queryFn: () => api.getMe(),
    retry: false,
    staleTime: Infinity,
  });
  return {
    user: data ?? null,
    isLoading,
    login: () => {
      window.location.assign(browserFlowURL("login", window.location.pathname));
    },
    logout: async () => {
      window.location.assign(await createBrowserLogoutURL("/login"));
    },
  };
}

function PluginAppHeaderButton(props: Record<string, unknown>) {
  const Comp = getTos().windows.AppHeaderButton;
  return <Comp {...(props as Record<string, never>)} />;
}

export const pluginShim: HostShim = {
  useNavigationPort: () => getTos().navigation,
  useAppHeaderSlot: () => getTos().windows.useAppHeaderSlot(),
  AppHeaderButton: PluginAppHeaderButton,
  useInstancePersistentState: (instanceId, key, defaultFn) =>
    getTos().workspace.useInstancePersistentState(instanceId, key, defaultFn),
  useInstanceState: (instanceId, key, defaultFn) =>
    getTos().workspace.useInstanceState(instanceId, key, defaultFn),
  useCurrentUser,
  useShellContext: () => pluginFallbackContext,
};
