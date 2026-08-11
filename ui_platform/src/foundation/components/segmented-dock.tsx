import { Columns3, Square } from "lucide-react";
import { SlotPicker } from "@/foundation/components/slot-picker";
import { useSlots } from "@/foundation/providers/slot-provider";

export function SegmentedDock() {
  const { apps, configs, profile, setLayout, activateInstance } = useSlots();
  return (
    <footer className="flex h-14 shrink-0 items-center justify-center px-3 pb-2">
      <nav
        aria-label="Application dock"
        className="flex max-w-full items-center gap-1 overflow-x-auto rounded-full border border-slate-200 bg-white/85 p-1 shadow-lg backdrop-blur"
      >
        <button
          type="button"
          aria-label="Switch layout"
          onClick={() =>
            setLayout(profile.layoutId === "3col" ? "1col" : "3col")
          }
          className="rounded-full p-2 text-slate-500 hover:bg-slate-100"
        >
          {profile.layoutId === "3col" ? (
            <Columns3 size={15} />
          ) : (
            <Square size={15} />
          )}
        </button>
        {configs.map((config) => {
          const state = profile.slots[config.id];
          return (
            <div key={config.id} className="flex items-center gap-1">
              <span className="mx-1 h-5 w-px bg-slate-200" />
              {state.opened.map((instance) => {
                const app = apps.find((item) => item.id === instance.appId);
                return app ? (
                  <button
                    key={instance.id}
                    type="button"
                    onClick={() => activateInstance(config.id, instance.id)}
                    className={`rounded-full px-3 py-1.5 text-xs font-medium ${profile.activeSlotId === config.id && state.activeInstanceId === instance.id ? "bg-slate-900 text-white" : "text-slate-500 hover:bg-slate-100"}`}
                  >
                    {app.label}
                  </button>
                ) : null;
              })}
              <SlotPicker slotId={config.id}>
                <PlusGlyph />
              </SlotPicker>
            </div>
          );
        })}
      </nav>
    </footer>
  );
}

function PlusGlyph() {
  return <span className="text-base leading-none">+</span>;
}
