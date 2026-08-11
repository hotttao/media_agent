import { Maximize2, Minimize2, X } from "lucide-react";
import { createContext, type ReactNode, useContext, useState } from "react";

const AppHeaderContext = createContext<HTMLDivElement | null>(null);
export function useAppHeaderSlot() {
  return useContext(AppHeaderContext);
}

export function AppWindow({
  slotId,
  filled,
  onFill,
  onClose,
  children,
}: {
  slotId: string;
  filled: boolean;
  onFill(): void;
  onClose(): void;
  children: ReactNode;
}) {
  const [headerSlot, setHeaderSlot] = useState<HTMLDivElement | null>(null);
  return (
    <section
      data-testid={`slot-${slotId}`}
      className="group flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white/90 shadow-sm backdrop-blur"
    >
      <header className="flex h-11 shrink-0 items-center gap-2 border-b border-slate-100 bg-slate-50/70 px-2">
        <div
          ref={setHeaderSlot}
          className="flex min-w-0 flex-1 items-center gap-2"
        />
        <div className="flex items-center gap-0.5 opacity-70 transition-opacity group-hover:opacity-100">
          <button
            type="button"
            aria-label={filled ? "Restore layout" : "Fill slot"}
            onClick={onFill}
            className="rounded-lg p-2 text-slate-500 hover:bg-white hover:text-slate-900"
          >
            {filled ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
          </button>
          <button
            type="button"
            aria-label="Close app"
            onClick={onClose}
            className="rounded-lg p-2 text-slate-500 hover:bg-red-50 hover:text-red-600"
          >
            <X size={14} />
          </button>
        </div>
      </header>
      <AppHeaderContext.Provider value={headerSlot}>
        <div className="flex min-h-0 flex-1 flex-col">{children}</div>
      </AppHeaderContext.Provider>
    </section>
  );
}
