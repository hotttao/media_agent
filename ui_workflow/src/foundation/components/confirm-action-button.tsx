import { useState, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";

export function ConfirmActionButton({
  onConfirm,
  children,
  ...props
}: Omit<ComponentProps<typeof Button>, "onClick"> & {
  onConfirm: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  return (
    <>
      <Button
        {...props}
        onClick={() => {
          setError("");
          setOpen(true);
        }}
      >
        {children}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!pending) setOpen(value);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("actionConfirm.title")}</DialogTitle>
            <DialogDescription>{t("actionConfirm.hint")}</DialogDescription>
          </DialogHeader>
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={pending}
              onClick={async () => {
                setPending(true);
                try {
                  await onConfirm();
                  setOpen(false);
                } catch (cause) {
                  setError(
                    cause instanceof Error
                      ? cause.message
                      : t("actionConfirm.error"),
                  );
                } finally {
                  setPending(false);
                }
              }}
            >
              {t("actionConfirm.proceed")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
