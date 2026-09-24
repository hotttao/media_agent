import { ImagePlus, Loader2, Star, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { LibraryImage } from "@/foundation/components/library-image";
import { uploadLibraryObject } from "@/foundation/lib/library-object";
import type { ProductImageInput } from "../types";

export function normalizeProductImages(
  images: ProductImageInput[],
): ProductImageInput[] {
  return images.map((image, index) => ({
    ...image,
    sort_order: index,
    is_main: images.some((x) => x.is_main) ? image.is_main : index === 0,
  }));
}

export function ProductImagesField({
  organizationId,
  scope,
  value,
  onChange,
  onBusyChange,
}: {
  organizationId: string;
  scope: string;
  value: ProductImageInput[];
  onChange: (images: ProductImageInput[]) => void;
  onBusyChange?: (busy: boolean) => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const canUpload = Boolean(organizationId && scope.trim());

  async function select(files: FileList | null) {
    if (!files?.length) return;
    if (!organizationId || !scope.trim()) {
      setError(t("productImages.scopeFirst"));
      return;
    }
    setBusy(true);
    onBusyChange?.(true);
    setError(undefined);
    try {
      const uploaded = await Promise.all(
        Array.from(files).map(async (file) => {
          if (!file.type.startsWith("image/") || file.size > 4 * 1024 * 1024)
            throw new Error(
              t("productImages.invalidFile", { name: file.name }),
            );
          const result = await uploadLibraryObject({
            organizationId,
            scope,
            purpose: "product_image",
            file,
          });
          return {
            object_path: result.object_path,
            is_main: false,
            sort_order: 0,
          };
        }),
      );
      onChange(normalizeProductImages([...value, ...uploaded]));
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("productImages.failed"),
      );
    } finally {
      setBusy(false);
      onBusyChange?.(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  return (
    <div className="grid gap-3">
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("productImages.title")}</Label>
          <p className="text-xs text-muted-foreground">
            {t("productImages.hint")}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => inputRef.current?.click()}
          disabled={busy || !canUpload}
        >
          {busy ? (
            <Loader2 className="mr-1 h-4 w-4 animate-spin" />
          ) : (
            <ImagePlus className="mr-1 h-4 w-4" />
          )}
          {t("productImages.add")}
        </Button>
      </div>
      <input
        ref={inputRef}
        type="file"
        multiple
        accept="image/jpeg,image/png,image/webp,image/gif"
        className="sr-only"
        onChange={(e) => void select(e.target.files)}
      />
      {value.length ? (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
          {value.map((image, index) => (
            <div
              key={`${image.object_path}-${index}`}
              className="overflow-hidden rounded-xl border"
            >
              <div className="aspect-square">
                <LibraryImage
                  organizationId={organizationId}
                  objectPath={image.object_path}
                  alt={t("productImages.imageAlt", { index: index + 1 })}
                />
              </div>
              <div className="flex items-center justify-between border-t p-2">
                <Button
                  type="button"
                  variant={image.is_main ? "secondary" : "ghost"}
                  size="sm"
                  onClick={() =>
                    onChange(
                      value.map((item, itemIndex) => ({
                        ...item,
                        is_main: itemIndex === index,
                      })),
                    )
                  }
                >
                  <Star
                    className={`mr-1 h-4 w-4 ${image.is_main ? "fill-current" : ""}`}
                  />
                  {t(
                    image.is_main
                      ? "productImages.main"
                      : "productImages.setMain",
                  )}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() =>
                    onChange(
                      normalizeProductImages(
                        value.filter((_, itemIndex) => itemIndex !== index),
                      ),
                    )
                  }
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <button
          type="button"
          className="flex h-32 items-center justify-center rounded-xl border border-dashed text-sm text-muted-foreground"
          onClick={() => inputRef.current?.click()}
          disabled={!canUpload}
        >
          <ImagePlus className="mr-2 h-5 w-5" />
          {t(canUpload ? "productImages.upload" : "uploadField.scopeRequired")}
        </button>
      )}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  );
}
