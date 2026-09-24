import { ImagePlus, Loader2, RefreshCw, Trash2, Upload } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { cn } from "@/foundation/lib/utils";
import {
  type LibraryObjectPurpose,
  uploadLibraryObject,
} from "../lib/library-object";
import { LibraryImage } from "./library-image";

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
}) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState<string>();
  const [file, setFile] = useState<File>();
  const [key, setKey] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const canUpload = Boolean(organizationId && scope.trim());

  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );

  async function upload(nextFile: File, idempotencyKey: string) {
    if (!organizationId || !scope.trim()) {
      setError("请先填写 Scope，再选择图片");
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
      setError(cause instanceof Error ? cause.message : "图片上传失败");
    } finally {
      setBusy(false);
      onBusyChange?.(false);
    }
  }

  function selectFile(nextFile?: File) {
    if (!nextFile) return;
    if (!ACCEPTED_TYPES.includes(nextFile.type)) {
      setError("仅支持 JPEG、PNG、WebP 或 GIF 图片");
      return;
    }
    if (nextFile.size > MAX_BYTES) {
      setError("图片不能超过 4 MiB");
      return;
    }
    if (preview) URL.revokeObjectURL(preview);
    const nextPreview = URL.createObjectURL(nextFile);
    const nextKey = crypto.randomUUID();
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
        accept={ACCEPTED_TYPES.join(",")}
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
            <img
              src={preview}
              alt={`${label} local preview`}
              className="h-full w-full object-cover"
            />
          ) : value ? (
            <LibraryImage
              organizationId={organizationId}
              objectPath={value}
              alt={label}
            />
          ) : (
            <span className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
              <ImagePlus className="h-8 w-8" />
              点击选择图片
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
              ? "正在上传…"
              : value
                ? "已保存到素材存储"
                : canUpload
                  ? "尚未选择图片"
                  : "请先填写 Scope"}
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
              {hasImage ? "替换" : "上传"}
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
