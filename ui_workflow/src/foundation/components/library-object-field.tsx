import { ImagePlus, Loader2, RefreshCw, Trash2, Upload } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { cn } from "@/foundation/lib/utils";
import { requestKey } from "@/foundation/lib/request-key";
import {
  type LibraryObjectPurpose,
  uploadLibraryObject,
} from "../lib/library-object";
import { LibraryImage, LibraryVideo } from "./library-image";

const ACCEPTED_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif"];
const MAX_BYTES = 4 * 1024 * 1024;

export function LibraryObjectField({
  organizationId,
  scope,
  purpose,
  label,
  hint,
  value,
  onChange,
  onBusyChange,
  className,
  acceptVideo = false,
}: {
  organizationId: string;
  scope: string;
  purpose: LibraryObjectPurpose;
  label: string;
  hint?: string;
  value?: string;
  onChange: (objectPath: string) => void;
  onBusyChange?: (busy: boolean) => void;
  className?: string;
  acceptVideo?: boolean;
}) {
  const { t } = useTranslation();
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState<string>();
  const [file, setFile] = useState<File>();
  const [key, setKey] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const canUpload = Boolean(organizationId && scope.trim());
  const acceptedTypes = acceptVideo
    ? ["video/mp4", "video/webm"]
    : ACCEPTED_TYPES;

  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );

  async function upload(nextFile: File, idempotencyKey: string) {
    if (!organizationId || !scope.trim()) {
      setError(t("uploadField.scopeFirst"));
      return;
    }
    setBusy(true);
    onBusyChange?.(true);
    setError(undefined);
    try {
      const uploaded = await uploadLibraryObject({
        organizationId,
        scope: scope.trim(),
        purpose,
        file: nextFile,
        idempotencyKey,
      });
      onChange(uploaded.object_path);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("uploadField.failed"),
      );
    } finally {
      setBusy(false);
      onBusyChange?.(false);
    }
  }

  function selectFile(nextFile?: File) {
    if (!nextFile) return;
    if (!acceptedTypes.includes(nextFile.type)) {
      setError(
        acceptVideo ? t("uploadField.videoTypes") : t("uploadField.imageTypes"),
      );
      return;
    }
    if (nextFile.size > (acceptVideo ? 16 * 1024 * 1024 : MAX_BYTES)) {
      setError(
        t(acceptVideo ? "uploadField.videoSize" : "uploadField.imageSize"),
      );
      return;
    }
    if (preview) URL.revokeObjectURL(preview);
    const nextPreview = URL.createObjectURL(nextFile);
    const nextKey = requestKey();
    setPreview(nextPreview);
    setFile(nextFile);
    setKey(nextKey);
    void upload(nextFile, nextKey);
  }

  const hasImage = Boolean(preview || value);
  return (
    <div className={cn("grid gap-2", className)}>
      <div>
        <Label htmlFor={inputId}>{label}</Label>
        {hint ? (
          <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
        ) : null}
      </div>
      <input
        ref={inputRef}
        id={inputId}
        type="file"
        accept={acceptedTypes.join(",")}
        className="sr-only"
        onChange={(event) => selectFile(event.target.files?.[0])}
      />
      <div className="overflow-hidden rounded-xl border bg-muted/20">
        <button
          type="button"
          className="relative block aspect-[4/3] w-full overflow-hidden text-left"
          onClick={() => inputRef.current?.click()}
          disabled={busy || !canUpload}
        >
          {preview ? (
            acceptVideo ? (
              <video
                src={preview}
                className="h-full w-full object-contain"
                muted
              />
            ) : (
              <img
                src={preview}
                alt={`${label} ${t("uploadField.localPreview")}`}
                className="h-full w-full object-cover"
              />
            )
          ) : value ? (
            acceptVideo ? (
              <LibraryVideo
                organizationId={organizationId}
                objectPath={value}
                controls={false}
              />
            ) : (
              <LibraryImage
                organizationId={organizationId}
                objectPath={value}
                alt={label}
              />
            )
          ) : (
            <span className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
              <ImagePlus className="h-8 w-8" />
              {t(
                acceptVideo
                  ? "uploadField.chooseVideo"
                  : "uploadField.chooseImage",
              )}
            </span>
          )}
          {busy ? (
            <span className="absolute inset-0 flex items-center justify-center bg-background/75">
              <Loader2 className="h-6 w-6 animate-spin" />
            </span>
          ) : null}
        </button>
        <div className="flex items-center justify-between gap-2 border-t p-2">
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {busy
              ? t("uploadField.uploading")
              : value
                ? t("uploadField.saved")
                : canUpload
                  ? t("uploadField.notSelected")
                  : t("uploadField.scopeRequired")}
          </span>
          <div className="flex shrink-0 gap-1">
            {error && file && key ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => void upload(file, key)}
              >
                <RefreshCw className="h-4 w-4" />
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => inputRef.current?.click()}
              disabled={busy || !canUpload}
            >
              <Upload className="mr-1 h-4 w-4" />
              {t(hasImage ? "uploadField.replace" : "uploadField.upload")}
            </Button>
            {hasImage ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => {
                  if (preview) URL.revokeObjectURL(preview);
                  setPreview(undefined);
                  setFile(undefined);
                  setKey(undefined);
                  setError(undefined);
                  onChange("");
                  if (inputRef.current) inputRef.current.value = "";
                }}
                disabled={busy}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
        </div>
      </div>
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  );
}
