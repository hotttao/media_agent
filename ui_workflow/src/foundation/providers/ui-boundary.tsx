import { type ReactNode, useState } from "react";
import { createPortal } from "react-dom";
import { PortalContainerContext } from "@/components/ui/portal-context";
import { useFullscreenContainer } from "@/foundation/hooks/useFullscreenContainer";

/** Shared by standalone and panels; never changes host theme or body styles. */
export function UiBoundary({ children }: { children: ReactNode }) {
  const fullscreen = useFullscreenContainer();
  const [portal, setPortal] = useState<HTMLDivElement | null>(null);
  return (
    <PortalContainerContext.Provider value={portal}>
      <div className="media-workflow-ui" data-media-ui="app">
        {children}
      </div>
      {createPortal(
        <div
          className="media-workflow-ui media-workflow-portal"
          data-media-ui="portal"
          ref={setPortal}
        />,
        fullscreen ?? document.body,
      )}
    </PortalContainerContext.Provider>
  );
}
