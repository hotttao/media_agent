import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { StructuredObjectEditor } from "@/foundation/components/structured-object-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { hostShim } from "@/foundation/lib/host-shim";
import type { ProductInput } from "../types";
import { ProductImagesField } from "./product-images-field";

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
  return (
    <div className="flex flex-col gap-6">
      <AppStudioHeader>
        <h1 className="truncate text-base font-semibold">
          {value.name || submitLabel}
        </h1>{" "}
        <div className="flex gap-2">
          <Button
            onClick={onSubmit}
            disabled={
              isPending ||
              uploading ||
              !value.name.trim() ||
              !value.scope.trim()
            }
          >
            {uploading ? t("materialForm.uploading") : submitLabel}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
        </div>
      </AppStudioHeader>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("productForm.productInfo")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("productForm.productInfoHint")}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>{t("v12.scope")}</Label>
            <Input
              disabled={isEdit}
              value={value.scope}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.name")}</Label>
            <Input
              aria-label={t("materialForm.name")}
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
          <StructuredObjectEditor
            value={value.external_commerce}
            onChange={(external_commerce) =>
              onChange({ ...value, external_commerce })
            }
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("materialDetail.metadata")}</Label>
          <StructuredObjectEditor
            value={value.metadata}
            onChange={(metadata) => onChange({ ...value, metadata })}
          />
        </div>
      </section>
    </div>
  );
}
