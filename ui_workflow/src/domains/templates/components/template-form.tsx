import { Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  defaultImageInvocation,
  updateTemplateSlot,
  providerIdentity,
} from "@/domains/templates/lib/invocation-editing";
import { useAssetKinds } from "@/domains/materials/hooks/use-asset-kinds";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
  TemplateInput,
  TemplateSlotInput,
  TemplateSlotResourceType,
  Visibility,
} from "@/domains/templates/types";

const SLOT_RESOURCE_TYPES: TemplateSlotResourceType[] = [
  "MATERIAL",
  "ASSET",
  "PRODUCT",
  "VIRTUAL_IP",
  "TEXT",
];

const VISIBILITY_OPTIONS: Visibility[] = ["private", "team", "public"];

function emptySlot(): TemplateSlotInput {
  return { name: "", asset_kind: "", resource_type: "ASSET", required: false };
}

/**
 * Shared form for create/edit. Caller owns state + submit mutation.
 *
 * inputs is a dynamic list: each slot has name + resource_type (select) +
 * required (checkbox). display_order is derived from list index on submit
 * (caller can post-process if needed). output_asset_kind is a select of ASSET
 * resource kinds (per docs: output_asset_kind must reference an
 * asset_kind with resource_type=ASSET).
 *
 * visibility is included for UX continuity; the create flow is expected to
 * call changeTemplateVisibility after createTemplate when a non-default value
 * is selected (the proto's CreateTemplateReq does not carry visibility).
 */
export function TemplateForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  scopeLocked = false,
}: {
  value: TemplateInput;
  onChange: (v: TemplateInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  scopeLocked?: boolean;
}) {
  const { t } = useTranslation();
  const { data: assetKinds } = useAssetKinds();

  function updateSlot(index: number, patch: Partial<TemplateSlotInput>) {
    onChange(updateTemplateSlot(value, index, patch));
  }

  function addSlot() {
    const next = [...value.inputs, emptySlot()];
    onChange({ ...value, inputs: next });
  }

  function removeSlot(index: number) {
    const next = value.inputs.filter((_, i) => i !== index);
    onChange({ ...value, inputs: next });
  }

  return (
    <div className="template-workbench">
      <div className="grid gap-2">
        <Label>{t("v12.scope")}</Label>
        <Input
          value={value.scope}
          disabled={scopeLocked}
          onChange={(e) => onChange({ ...value, scope: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.name")}</Label>
        <Input
          value={value.name}
          onChange={(e) => onChange({ ...value, name: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.description")}</Label>
        <Textarea
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
      </div>
      <div className="template-prompt grid gap-2">
        <Label>{t("templateFields.prompt")}</Label>
        <Textarea
          className="font-mono text-xs"
          rows={10}
          value={value.prompt_body}
          onChange={(e) => onChange({ ...value, prompt_body: e.target.value })}
          placeholder={t("templateFields.promptHint", { example: "{{.name}}" })}
        />
      </div>

      <div className="template-slots grid gap-2">
        <div className="flex items-center justify-between">
          <Label>{t("v12.inputs")}</Label>
          <Button type="button" variant="outline" size="sm" onClick={addSlot}>
            {t("templateFields.addSlot")}
          </Button>
        </div>
        <div className="flex flex-col gap-2">
          {value.inputs.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t("templateFields.noSlots")}
            </p>
          ) : null}
          {value.inputs.map((slot, i) => (
            <div
              key={i}
              className="flex flex-wrap items-end gap-2 rounded-xl bg-muted/40 p-3"
            >
              <div className="grid flex-1 gap-1">
                <Label className="text-xs">
                  {t("templateFields.slotName")}
                </Label>
                <Input
                  value={slot.name}
                  onChange={(e) => updateSlot(i, { name: e.target.value })}
                />
              </div>
              <div className="grid w-40 gap-1">
                <Label className="text-xs">
                  {t("templateFields.resourceType")}
                </Label>
                <Select
                  value={slot.resource_type}
                  onValueChange={(v) => updateSlot(i, { resource_type: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SLOT_RESOURCE_TYPES.map((t) => (
                      <SelectItem key={t} value={t}>
                        {t}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {slot.resource_type ? (
                <div className="grid w-44 gap-1">
                  <Label className="text-xs">
                    {t("templateFields.assetKind")}
                  </Label>
                  <Select
                    value={slot.asset_kind}
                    onValueChange={(assetKind) =>
                      updateSlot(i, { asset_kind: assetKind })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t("templateFields.assetKind")}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {(assetKinds ?? [])
                        .filter(
                          (kind) =>
                            String(kind.resource_type) === slot.resource_type ||
                            String(kind.resource_type) ===
                              (
                                {
                                  MATERIAL: "1",
                                  ASSET: "2",
                                  PRODUCT: "3",
                                  VIRTUAL_IP: "4",
                                  TEXT: "5",
                                } as Record<string, string>
                              )[slot.resource_type],
                        )
                        .map((kind) => (
                          <SelectItem key={kind.name} value={kind.name}>
                            {kind.name}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : null}
              <div className="flex items-center gap-2 pb-2">
                <Checkbox
                  id={`slot-required-${i}`}
                  checked={slot.required}
                  onCheckedChange={(v) =>
                    updateSlot(i, { required: v === true })
                  }
                />
                <Label htmlFor={`slot-required-${i}`} className="text-xs">
                  {t("templateFields.required")}
                </Label>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t("templateFields.removeSlot")}
                onClick={() => removeSlot(i)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="template-capability-key">
          {t("templateEditor.capability")}
        </Label>
        <Input
          id="template-capability-key"
          value={value.capability_key}
          placeholder="jimeng.i2i.multi_image_prompt"
          onChange={(event) =>
            onChange({
              ...value,
              capability_key: event.target.value,
              ...providerIdentity(event.target.value),
            })
          }
        />
        <Label htmlFor="template-capability-version">{t("v12.version")}</Label>
        <Input
          id="template-capability-version"
          type="number"
          min={1}
          step={1}
          value={value.capability_version}
          onChange={(event) =>
            onChange({
              ...value,
              capability_version: Number(event.target.value),
            })
          }
        />
        <p className="text-xs text-muted-foreground">
          {t("templateEditor.catalogUnavailable")}
        </p>
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            onChange({
              ...value,
              invocation_mapping: defaultImageInvocation(value.inputs),
            })
          }
        >
          {t("templateEditor.resetMapping")}
        </Button>
        <p className="text-xs text-muted-foreground">
          {t("templateEditor.mappingHint")}
        </p>
      </div>

      <div className="grid gap-2">
        <Label>{t("templateFields.outputKind")}</Label>
        <Select
          value={value.output_asset_kind}
          onValueChange={(v) => onChange({ ...value, output_asset_kind: v })}
        >
          <SelectTrigger>
            <SelectValue placeholder={t("templateFields.outputKind")} />
          </SelectTrigger>
          <SelectContent>
            {(assetKinds ?? [])
              .filter(
                (kind) =>
                  kind.resource_type === "ASSET" ||
                  String(kind.resource_type) === "2",
              )
              .map((k) => (
                <SelectItem key={k.name} value={k.name}>
                  {k.name}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.platform")}</Label>
        <Input value={value.platform} readOnly />
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.generationMethod")}</Label>
        <Input value={value.gen_way} readOnly />
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.generationType")}</Label>
        <Input value={value.gen_type ?? ""} readOnly />
      </div>
      <div className="flex items-center gap-2">
        <Checkbox
          id="needs-confirm"
          checked={value.needs_confirm ?? false}
          onCheckedChange={(v) =>
            onChange({ ...value, needs_confirm: v === true })
          }
        />
        <Label htmlFor="needs-confirm">{t("templateFields.review")}</Label>
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.candidates")}</Label>
        <Input
          type="number"
          value={String(value.candidate_count ?? 0)}
          onChange={(e) =>
            onChange({ ...value, candidate_count: Number(e.target.value) })
          }
        />
      </div>
      <div className="grid gap-2">
        <Label>{t("templateFields.visibility")}</Label>
        <Select
          value={value.visibility ?? "private"}
          onValueChange={(v) =>
            onChange({ ...value, visibility: v as Visibility })
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {VISIBILITY_OPTIONS.map((v) => (
              <SelectItem key={v} value={v}>
                {t(`common.visibility.${v}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="template-actions flex gap-2">
        <Button
          onClick={onSubmit}
          disabled={
            isPending ||
            !value.name ||
            !value.scope ||
            !value.capability_key ||
            !value.output_asset_kind ||
            !Number.isInteger(value.capability_version) ||
            value.capability_version < 1
          }
        >
          {submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}
