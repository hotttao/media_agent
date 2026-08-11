import { Plus, Search } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useSlots } from "@/foundation/providers/slot-provider";

export function SlotPicker({
  slotId,
  children,
}: {
  slotId: string;
  children?: ReactNode;
}) {
  const { apps, openApp } = useSlots();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);
  const visible = apps.filter(
    (app) =>
      !app.hidden &&
      `${app.label} ${app.id}`.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div ref={root} className="relative">
      <button
        type="button"
        aria-label={`Open app in ${slotId}`}
        onClick={() => setOpen((value) => !value)}
        className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1.5 text-xs font-medium text-slate-500 hover:bg-slate-100 hover:text-slate-900"
      >
        {children ?? (
          <>
            <Plus size={14} />
            Add app
          </>
        )}
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-50 mb-2 w-64 overflow-hidden rounded-2xl border bg-white p-1 shadow-xl">
          <div className="flex items-center gap-2 border-b px-2">
            <Search size={14} className="text-slate-400" />
            <input
              aria-label="Search apps"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search apps…"
              className="h-10 min-w-0 flex-1 text-sm outline-none"
            />
          </div>
          <div className="max-h-64 overflow-y-auto py-1">
            {visible.map((app) => (
              <button
                key={app.id}
                type="button"
                onClick={() => {
                  openApp(slotId, app.id);
                  setOpen(false);
                  setQuery("");
                }}
                className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm hover:bg-slate-100"
              >
                <span>{app.label}</span>
                <span className="text-xs text-slate-400">Open</span>
              </button>
            ))}
            {!visible.length && (
              <p className="px-3 py-6 text-center text-xs text-slate-500">
                No apps found
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
