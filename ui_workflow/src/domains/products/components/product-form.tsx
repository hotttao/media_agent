import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { hostShim } from "@/foundation/lib/host-shim";
import type { ProductInput } from "../types";
import { ProductImagesField } from "./product-images-field";

function jsonText(value?: Record<string, unknown>) {
  return value && Object.keys(value).length
    ? JSON.stringify(value, null, 2)
    : "";
}
function listText(value?: unknown[]) {
  return value?.map(String).join("\n") ?? "";
}

export function ProductForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  isEdit,
}: {
  value: ProductInput;
  onChange: (v: ProductInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  isEdit?: boolean;
}) {
  const { t } = useTranslation();
  const { organization } = hostShim.useShellContext();
  const [uploading, setUploading] = useState(false);
  const [externalText, setExternalText] = useState(() =>
    jsonText(value.external_commerce),
  );
  const [metadataText, setMetadataText] = useState(() =>
    jsonText(value.metadata),
  );
  const [jsonError, setJsonError] = useState<Record<string, string>>({});
  function changeJSON(key: "external_commerce" | "metadata", text: string) {
    key === "metadata" ? setMetadataText(text) : setExternalText(text);
    if (!text.trim()) {
      setJsonError((old) => ({ ...old, [key]: "" }));
      onChange({ ...value, [key]: undefined });
      return;
    }
    try {
      const parsed = JSON.parse(text) as Record<string, unknown>;
      setJsonError((old) => ({ ...old, [key]: "" }));
      onChange({ ...value, [key]: parsed });
    } catch (error) {
      setJsonError((old) => ({
        ...old,
        [key]:
          error instanceof Error ? error.message : t("materialForm.jsonError"),
      }));
    }
  }
  return (
    <div className="flex flex-col gap-6">
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("productForm.productInfo")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("productForm.productInfoHint")}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>Scope</Label>
            <Input
              disabled={isEdit}
              value={value.scope}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.name")}</Label>
            <Input
              value={value.name}
              onChange={(e) => onChange({ ...value, name: e.target.value })}
            />
          </div>
        </div>
        <div className="grid gap-2">
          <Label>{t("materialForm.description")}</Label>
          <Textarea
            value={value.description ?? ""}
            onChange={(e) =>
              onChange({ ...value, description: e.target.value })
            }
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("productForm.audience")}</Label>
          <Textarea
            value={value.target_audience ?? ""}
            onChange={(e) =>
              onChange({ ...value, target_audience: e.target.value })
            }
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("productForm.details")}</Label>
          <Textarea
            rows={6}
            value={value.product_details ?? ""}
            onChange={(e) =>
              onChange({ ...value, product_details: e.target.value })
            }
          />
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <ProductImagesField
          organizationId={organization.id}
          scope={value.scope}
          value={value.images ?? []}
          onChange={(images) => onChange({ ...value, images })}
          onBusyChange={setUploading}
        />
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <h3 className="font-medium">{t("productForm.generation")}</h3>
        <div className="grid gap-2">
          <Label>{t("productForm.actions")}</Label>
          <Textarea
            value={listText(value.display_actions)}
            onChange={(e) =>
              onChange({
                ...value,
                display_actions: e.target.value
                  .split("\n")
                  .map((x) => x.trim())
                  .filter(Boolean),
              })
            }
            placeholder={t("productForm.actionsHint")}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("productForm.tags")}</Label>
          <Input
            value={value.tags?.join(", ") ?? ""}
            onChange={(e) =>
              onChange({
                ...value,
                tags: e.target.value
                  .split(/[,，]/)
                  .map((x) => x.trim())
                  .filter(Boolean),
              })
            }
            placeholder={t("productForm.tagsHint")}
          />
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <h3 className="font-medium">{t("productForm.extended")}</h3>
        <div className="grid gap-2">
          <Label>{t("productForm.externalCommerce")}</Label>
          <Textarea
            className="font-mono text-xs"
            rows={5}
            value={externalText}
            onChange={(e) => changeJSON("external_commerce", e.target.value)}
            placeholder="{}"
          />
          {jsonError.external_commerce ? (
            <p className="text-xs text-destructive">
              {jsonError.external_commerce}
            </p>
          ) : null}
        </div>
        <div className="grid gap-2">
          <Label>Metadata (JSON)</Label>
          <Textarea
            className="font-mono text-xs"
            rows={5}
            value={metadataText}
            onChange={(e) => changeJSON("metadata", e.target.value)}
            placeholder="{}"
          />
          {jsonError.metadata ? (
            <p className="text-xs text-destructive">{jsonError.metadata}</p>
          ) : null}
        </div>
      </section>
      <div className="flex gap-2">
        <Button
          onClick={onSubmit}
          disabled={
            isPending ||
            uploading ||
            Object.values(jsonError).some(Boolean) ||
            !value.name.trim()
          }
        >
          {uploading ? t("materialForm.uploading") : submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}
