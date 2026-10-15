import {
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
  useId,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ManagementError } from "@/foundation/lib/management-http";
export function CatalogError({
  error,
  retry,
}: {
  error: unknown;
  retry?(): void;
}) {
  const { t } = useTranslation();
  if (!error) return null;
  const code = error instanceof ManagementError ? error.status : 0;
  return (
    <div
      role="alert"
      className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800"
    >
      <p>
        {t(
          code === 403
            ? "catalog.forbidden"
            : code === 409
              ? "catalog.conflict"
              : code === 400 || code === 422
                ? "catalog.invalid"
                : "catalog.failed",
        )}
      </p>
      {retry && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={retry}
          className="mt-2"
        >
          {t("catalog.reload")}
        </Button>
      )}
    </div>
  );
}
export function CatalogModal({
  title,
  children,
  close,
}: {
  title: string;
  children: ReactNode;
  close(): void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        closeLabel={t("catalog.close")}
        className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}
export function ConfirmAction({
  title,
  description,
  action,
  disabled,
  onError,
}: {
  title: string;
  description: string;
  action(): Promise<void>;
  disabled?: boolean;
  onError(error: unknown): void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => {
          setError(undefined);
          setOpen(true);
        }}
      >
        {title}
      </Button>
      {open && (
        <CatalogModal
          title={title}
          close={() => {
            if (!busy) setOpen(false);
          }}
        >
          <p className="text-sm text-muted-foreground">{description}</p>
          <CatalogError error={error} />
          <div className="mt-4 flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => setOpen(false)}
            >
              {t("catalog.cancel")}
            </Button>
            <Button
              type="button"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                try {
                  await action();
                  setOpen(false);
                } catch (error) {
                  setError(error);
                  onError(error);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {t(busy ? "catalog.saving" : "catalog.confirm")}
            </Button>
          </div>
        </CatalogModal>
      )}
    </>
  );
}
export function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <div className="grid gap-1.5 text-sm">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      {isValidElement(children)
        ? cloneElement(children as ReactElement<{ id?: string }>, { id })
        : children}
    </div>
  );
}
export const inputClass =
  "min-w-0 w-full rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-amber-300 disabled:bg-stone-100 disabled:text-stone-500";
