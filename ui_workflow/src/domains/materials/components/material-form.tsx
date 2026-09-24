import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useAssetKinds } from "@/domains/materials/hooks/use-asset-kinds";
import type { MaterialInput, Visibility } from "@/domains/materials/types";
import { LibraryObjectField } from "@/foundation/components/library-object-field";
import { hostShim } from "@/foundation/lib/host-shim";

function jsonText(value?: Record<string, unknown>) {
  return value && Object.keys(value).length
    ? JSON.stringify(value, null, 2)
    : "";
}

export function MaterialForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  isEdit,
}: {
  value: MaterialInput;
  onChange: (v: MaterialInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  isEdit?: boolean;
}) {
  const { t } = useTranslation();
  const { organization } = hostShim.useShellContext();
  const { data: kinds } = useAssetKinds("MATERIAL");
  const [uploading, setUploading] = useState(false);
  const [metadataText, setMetadataText] = useState(() =>
    jsonText(value.metadata),
  );
  const [metadataError, setMetadataError] = useState<string>();
  const isMovement = value.asset_kind === "movement";
  const metadata = value.metadata ?? {};

  function changeMetadata(text: string) {
    setMetadataText(text);
    if (!text.trim()) {
      setMetadataError(undefined);
      onChange({ ...value, metadata: undefined });
      return;
    }
    try {
      const parsed = JSON.parse(text) as Record<string, unknown>;
      setMetadataError(undefined);
      onChange({ ...value, metadata: parsed });
    } catch (error) {
      setMetadataError(
        error instanceof Error ? error.message : t("materialForm.jsonError"),
      );
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("materialForm.basic")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("materialForm.basicHint")}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>Scope</Label>
            <Input
              value={value.scope}
              disabled={isEdit}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.kind")}</Label>
            <Select
              disabled={isEdit}
              value={value.asset_kind}
              onValueChange={(asset_kind) =>
                onChange({ ...value, asset_kind, object_path: "" })
              }
            >
              <SelectTrigger>
                <SelectValue placeholder={t("materialForm.chooseKind")} />
              </SelectTrigger>
              <SelectContent>
                {(kinds ?? []).map((kind) => (
                  <SelectItem key={kind.name} value={kind.name}>
                    {t(`materialsList.kind.${kind.name}`, {
                      defaultValue: kind.name,
                    })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="grid gap-2">
          <Label>{t("materialForm.name")}</Label>
          <Input
            value={value.name}
            onChange={(e) => onChange({ ...value, name: e.target.value })}
          />
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
          <Label>{t("materialForm.prompt")}</Label>
          <Textarea
            value={value.prompt ?? ""}
            onChange={(e) => onChange({ ...value, prompt: e.target.value })}
            placeholder={t("materialForm.promptHint")}
          />
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("materialForm.contentTitle")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("materialForm.contentHint")}
          </p>
        </div>
        <LibraryObjectField
          organizationId={organization.id}
          scope={value.scope}
          purpose="material"
          label={t(isMovement ? "materialForm.video" : "materialForm.image")}
          acceptVideo={isMovement}
          value={value.object_path}
          onChange={(object_path) => onChange({ ...value, object_path })}
          onBusyChange={setUploading}
          className="max-w-xl"
        />
        <div className="grid gap-2">
          <Label>
            {t(
              isMovement
                ? "materialForm.movementContent"
                : "materialForm.textContent",
            )}
          </Label>
          <Textarea
            rows={6}
            value={value.content ?? ""}
            onChange={(e) => onChange({ ...value, content: e.target.value })}
          />
        </div>
      </section>
      {isMovement && (
        <section className="grid gap-4 rounded-xl border p-4">
          <h3 className="font-medium">{t("materialForm.movementTitle")}</h3>
          <div className="grid gap-2">
            <Label>{t("materialForm.applicableClothing")}</Label>
            <Input
              value={String(metadata.applicable_clothing ?? "")}
              onChange={(event) =>
                onChange({
                  ...value,
                  metadata: {
                    ...metadata,
                    applicable_clothing: event.target.value,
                  },
                })
              }
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.clothing")}</Label>
            <Input
              value={String(metadata.clothing ?? "")}
              onChange={(event) =>
                onChange({
                  ...value,
                  metadata: { ...metadata, clothing: event.target.value },
                })
              }
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.applicability")}</Label>
            <Select
              disabled={isEdit && metadata.is_general === false}
              value={metadata.is_general === false ? "specific" : "general"}
              onValueChange={(next) =>
                onChange({
                  ...value,
                  metadata: { ...metadata, is_general: next === "general" },
                })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="general">
                  {t("materialForm.general")}
                </SelectItem>
                <SelectItem value="specific" disabled>
                  {t("materialForm.specific")}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t("materialForm.poseLinkHint")}
            </p>
          </div>
        </section>
      )}
      <section className="grid gap-4 rounded-xl border p-4">
        <h3 className="font-medium">{t("materialForm.sharing")}</h3>
        <div className="grid gap-2">
          <Label>{t("materialForm.visibility")}</Label>
          <Select
            value={value.visibility ?? "private"}
            onValueChange={(visibility) =>
              onChange({ ...value, visibility: visibility as Visibility })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="private">
                {t("common.visibility.private")}
              </SelectItem>
              <SelectItem value="team">
                {t("common.visibility.team")}
              </SelectItem>
              <SelectItem value="public">
                {t("common.visibility.public")}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {!isMovement && (
          <div className="grid gap-2">
            <Label>Metadata (JSON)</Label>
            <Textarea
              className="font-mono text-xs"
              rows={6}
              value={metadataText}
              onChange={(e) => changeMetadata(e.target.value)}
              placeholder="{}"
            />
            {metadataError ? (
              <p className="text-xs text-destructive">{metadataError}</p>
            ) : null}
          </div>
        )}
      </section>
      <div className="flex gap-2">
        <Button
          onClick={onSubmit}
          disabled={
            isPending ||
            uploading ||
            Boolean(metadataError) ||
            !value.name.trim() ||
            (!value.object_path && !value.content?.trim())
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
