import { QueryClient } from "@tanstack/react-query";

/**
 * Shared QueryClient singleton.
 *
 * Standalone mounts it at the shell root (see src/main.tsx). Plugin mode
 * carries its own react-query copy + local QueryClient and mounts this inside
 * each panel (per docs/plan/ui_workflow.md §4.3 方案 C - plugin's query cache is
 * intentionally isolated from the host's).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
