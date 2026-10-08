import { createContext, useContext } from "react";

/** Portalled primitives stay inside the application's style boundary. */
export const PortalContainerContext = createContext<HTMLElement | null>(null);
export function usePortalContainer() {
  return useContext(PortalContainerContext);
}
