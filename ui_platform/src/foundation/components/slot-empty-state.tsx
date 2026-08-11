import { LayoutGrid } from "lucide-react";
import { SlotPicker } from "@/foundation/components/slot-picker";

export function SlotEmptyState({ slotId }: { slotId: string }) {
  return (
    <section
      data-testid={`slot-${slotId}`}
      className="grid min-h-0 flex-1 place-items-center rounded-2xl border border-dashed border-slate-300 bg-white/50 p-8 text-center"
    >
      <div>
        <LayoutGrid className="mx-auto mb-3 text-slate-400" size={24} />
        <p className="mb-3 text-sm text-slate-500">This slot is empty.</p>
        <SlotPicker slotId={slotId} />
      </div>
    </section>
  );
}
