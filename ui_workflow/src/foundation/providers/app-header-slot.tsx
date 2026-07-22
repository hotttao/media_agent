import { createContext, useContext } from "react";

/**
 * Portal target for app header content (buttons, breadcrumbs). The active
 * AppWindow provides the DOM node; apps read it via useAppHeaderSlot and
 * render into it with createPortal.
 *
 * Standalone mode: AppWindow (components/shell/windows/AppWindow.tsx) provides
 *   the node through this context.
 * Plugin mode: host-shim forwards useAppHeaderSlot to window.tos.windows
 *   instead - the host's AppWindow owns the slot, so this context is unused.
 */
export const AppHeaderSlotContext = createContext<HTMLDivElement | null>(null);

export function useAppHeaderSlot(): HTMLDivElement | null {
  return useContext(AppHeaderSlotContext);
}
