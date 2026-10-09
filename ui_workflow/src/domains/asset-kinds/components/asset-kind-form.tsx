import { Button } from "@/components/ui/button";
import { useTranslation } from "react-i18next";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
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
import type {
  AssetKindInput,
  AssetKindMediaType,
  AssetKindResourceType,
  AssetKindStatus,
} from "@/domains/asset-kinds/types";

const RESOURCE_TYPES: AssetKindResourceType[] = [
  "MATERIAL",
  "ASSET",
  "PRODUCT",
  "VIRTUAL_IP",
  "TEXT",
];
const MEDIA_TYPES: AssetKindMediaType[] = ["IMAGE", "TEXT", "VIDEO"];

/** Shared form for create/edit. Caller owns state + submit mutation.
 * `nameDisabled` is set in edit mode: name is the PK (path param) and cannot
 * be changed after creation (see UpdateAssetKindReq in asset_kind.proto). */
export function AssetKindForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  nameDisabled,
}: {
  value: AssetKindInput;
  onChange: (v: AssetKindInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  nameDisabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4">
      <AppStudioHeader>
        <h1 className="text-base font-semibold">
          {value.name || t("nav.assetKinds")}
        </h1>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onCancel} disabled={isPending}>
            {t("common.cancel")}
          </Button>
          <Button onClick={onSubmit} disabled={isPending || !value.name.trim()}>
            {submitLabel}
          </Button>
        </div>
      </AppStudioHeader>
      <div className="grid gap-2">
        <Label>{t("materialForm.name")}</Label>
        <Input
          aria-label={t("materialForm.name")}
          value={value.name}
          disabled={nameDisabled}
          onChange={(e) => onChange({ ...value, name: e.target.value })}
        />
      </div>
      {nameDisabled ? (
        <div className="grid gap-2">
          <Label>{t("registry.status")}</Label>
          <Select
            value={value.status ?? "ENABLED"}
            onValueChange={(status) =>
              onChange({ ...value, status: status as AssetKindStatus })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ENABLED">ENABLED</SelectItem>
              <SelectItem value="DISABLED">DISABLED</SelectItem>
              <SelectItem value="SYSTEM">SYSTEM</SelectItem>
            </SelectContent>
          </Select>
        </div>
      ) : null}
      <div className="grid gap-2">
        <Label>{t("registry.resourceType")}</Label>
        <Select
          disabled={nameDisabled}
          value={value.resource_type}
          onValueChange={(v) =>
            onChange({ ...value, resource_type: v as AssetKindResourceType })
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RESOURCE_TYPES.map((rt) => (
              <SelectItem key={rt} value={rt}>
                {rt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label>{t("registry.mediaType")}</Label>
        <Select
          disabled={nameDisabled}
          value={value.media_type}
          onValueChange={(v) =>
            onChange({ ...value, media_type: v as AssetKindMediaType })
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {MEDIA_TYPES.map((mt) => (
              <SelectItem key={mt} value={mt}>
                {mt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label>{t("materialForm.description")}</Label>
        <Textarea
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
      </div>
    </div>
  );
}
