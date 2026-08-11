import { AppWindow } from "@/foundation/components/app-window";
import { SlotEmptyState } from "@/foundation/components/slot-empty-state";
import { useSlots } from "@/foundation/providers/slot-provider";

export function SlotContainer({ slotId }: { slotId: string }) {
  const { apps, profile, closeInstance, filledSlot, setFilledSlot } =
    useSlots();
  const slot = profile.slots[slotId];
  if (!slot) return null;
  const active = slot.opened.find((item) => item.id === slot.activeInstanceId);
  const definition = apps.find((app) => app.id === active?.appId);
  const Component = definition?.Component;
  if (!active || !Component) return <SlotEmptyState slotId={slotId} />;
  const filled = filledSlot === slotId;
  return (
    <AppWindow
      slotId={slotId}
      filled={filled}
      onFill={() => setFilledSlot(filled ? null : slotId)}
      onClose={() => closeInstance(slotId, active.id)}
    >
      <Component key={active.id} instanceId={active.id} />
    </AppWindow>
  );
}
