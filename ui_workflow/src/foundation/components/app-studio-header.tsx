import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { hostShim } from "@/foundation/lib/host-shim";

/** Business toolbar only. The host still owns Slot chrome and window controls. */
export function AppStudioHeader({ children }: { children: ReactNode }) {
  const target = hostShim.useAppHeaderSlot();
  const content = (
    <div className="media-workflow-ui min-w-0 w-full">
      <div className="studio-header">{children}</div>
    </div>
  );
  return target ? createPortal(content, target) : content;
}
