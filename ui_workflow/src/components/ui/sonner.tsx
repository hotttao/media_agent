/**
 * Toaster bound to the local theme (standalone mode only). Plugin mode does
 * NOT render this - the host's Toaster covers notifications.
 *
 * Replaces agent-platform's @neutree-ai/theme ThemedToaster (internal monorepo
 * package not available here) with sonner's Toaster + local useTheme.
 */
import { useTheme } from "@/foundation/providers/theme-provider";
import { Toaster as SonnerToaster } from "sonner";

function ThemedToaster() {
  const { theme } = useTheme();
  return <SonnerToaster theme={theme} position="bottom-right" />;
}

export { ThemedToaster as Toaster };
