import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
export const Dialog = DialogPrimitive.Root;
export function DialogContent({
  children,
  className = "",
  closeLabel,
  ...props
}: ComponentPropsWithoutRef<typeof DialogPrimitive.Content> & {
  closeLabel?: string;
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-stone-950/35" />
      <DialogPrimitive.Content
        {...props}
        className={`fixed left-1/2 top-1/2 z-50 w-[calc(100%_-_2rem)] -translate-x-1/2 -translate-y-1/2 rounded-xl border bg-white p-6 shadow-xl ${className}`}
      >
        <DialogPrimitive.Description className="sr-only">
          {closeLabel}
        </DialogPrimitive.Description>
        {children}
        <DialogPrimitive.Close
          aria-label={closeLabel}
          className="absolute right-3 top-3 rounded p-1 text-stone-500 hover:bg-stone-100"
        >
          <X size={16} />
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}
export function DialogHeader({ children }: { children: ReactNode }) {
  return <div className="mb-4 pr-5">{children}</div>;
}
export function DialogTitle({ children }: { children: ReactNode }) {
  return (
    <DialogPrimitive.Title className="text-lg font-semibold tracking-tight">
      {children}
    </DialogPrimitive.Title>
  );
}
