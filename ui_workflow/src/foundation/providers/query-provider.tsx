/**
 * react-query provider. Both standalone (mounted at shell root) and plugin
 * (mounted inside each panel) use this with the shared queryClient singleton
 * (foundation/lib/query.ts).
 */
import { type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/foundation/lib/query";

export function QueryProvider({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
