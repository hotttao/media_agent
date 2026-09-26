import { SlotContainer } from "@/components/shell/SlotContainer";
import type { SlotConfig } from "@/foundation/providers/slot-provider";

/**
 * Single-slot layout — replaces the old "focus mode". slot-a takes the full
 * canvas; whatever app was in slot-a (or what the user opens here) is the
 * focus. Other layouts' slot-b / slot-c state is preserved off-screen.
 */
export const ONE_COLUMN_SLOTS: SlotConfig[] = [
  { id: "slot-a", defaultOpened: ["materials"] },
];

export function OneColumnLayout() {
  return (
    <div className="mx-auto flex h-full min-h-0 w-full max-w-[1840px] gap-0.5 p-2 sm:p-4">
      <div className="flex min-w-0 flex-1">
        <SlotContainer slotId="slot-a" />
      </div>
    </div>
  );
}
