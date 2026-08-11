import { useEffect, useState } from "react";
import { SlotContainer } from "@/foundation/components/slot-container";
import { useSlots } from "@/foundation/providers/slot-provider";

export function WorkspaceLayout() {
  const { profile, filledSlot } = useSlots();
  const [desktop, setDesktop] = useState(
    () => window.matchMedia("(min-width: 768px)").matches,
  );
  const [leftWidth, setLeftWidth] = useState(256);
  const [rightWidth, setRightWidth] = useState(448);

  useEffect(() => {
    const query = window.matchMedia("(min-width: 768px)");
    const update = () => setDesktop(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  function beginResize(
    side: "left" | "right",
    event: React.PointerEvent<HTMLButtonElement>,
  ) {
    event.preventDefault();
    const originX = event.clientX;
    const originWidth = side === "left" ? leftWidth : rightWidth;
    const move = (pointerEvent: PointerEvent) => {
      const delta = pointerEvent.clientX - originX;
      const next = side === "left" ? originWidth + delta : originWidth - delta;
      const bounded = Math.min(Math.max(next, 208), 560);
      if (side === "left") setLeftWidth(bounded);
      else setRightWidth(bounded);
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
  }

  function resizeWithKeyboard(side: "left" | "right", delta: number) {
    if (side === "left")
      setLeftWidth((value) => Math.min(Math.max(value + delta, 208), 560));
    else setRightWidth((value) => Math.min(Math.max(value + delta, 208), 560));
  }

  if (filledSlot)
    return (
      <div className="flex min-h-0 flex-1 p-3">
        <SlotContainer slotId={filledSlot} />
      </div>
    );
  return (
    <div className="flex min-h-0 flex-1 gap-2 p-3">
      {profile.layoutId === "3col" && desktop ? (
        <>
          <div className="flex min-h-0 shrink-0" style={{ width: leftWidth }}>
            <SlotContainer slotId="slot-a" />
          </div>
          <ResizeHandle
            label="Resize sessions panel"
            onPointerDown={(event) => beginResize("left", event)}
            onStep={(delta) => resizeWithKeyboard("left", delta)}
          />
          <div className="flex min-h-0 min-w-0 flex-1">
            <SlotContainer slotId="slot-b" />
          </div>
          <ResizeHandle
            label="Resize chat panel"
            onPointerDown={(event) => beginResize("right", event)}
            onStep={(delta) => resizeWithKeyboard("right", -delta)}
          />
          <div className="flex min-h-0 shrink-0" style={{ width: rightWidth }}>
            <SlotContainer slotId="slot-c" />
          </div>
        </>
      ) : null}
      {profile.layoutId !== "3col" || !desktop ? (
        <div className="flex min-h-0 min-w-0 flex-1">
          <SlotContainer slotId={profile.activeSlotId} />
        </div>
      ) : null}
    </div>
  );
}

function ResizeHandle({
  label,
  onPointerDown,
  onStep,
}: {
  label: string;
  onPointerDown(event: React.PointerEvent<HTMLButtonElement>): void;
  onStep(delta: number): void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onPointerDown={onPointerDown}
      onKeyDown={(event) => {
        if (event.key === "ArrowLeft") onStep(-16);
        if (event.key === "ArrowRight") onStep(16);
      }}
      className="group -mx-1 flex w-2 shrink-0 cursor-col-resize touch-none items-center justify-center"
    >
      <span className="h-10 w-0.5 rounded-full bg-slate-300 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
    </button>
  );
}
