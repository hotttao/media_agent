import { Trash2 } from "lucide-react";
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
  return { name: "", resource_type: "MATERIAL", required: false };
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
}: {
  value: TemplateInput;
  onChange: (v: TemplateInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
}) {
  const { data: assetKinds } = useAssetKinds("ASSET");

  function updateSlot(index: number, patch: Partial<TemplateSlotInput>) {
    const next = value.inputs.map((s, i) => (i === index ? { ...s, ...patch } : s));
    onChange({ ...value, inputs: next });
  }

  function addSlot() {
    onChange({ ...value, inputs: [...value.inputs, emptySlot()] });
  }

  function removeSlot(index: number) {
    onChange({ ...value, inputs: value.inputs.filter((_, i) => i !== index) });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-2">
        <Label>Scope</Label>
        <Input
          value={value.scope}
          onChange={(e) => onChange({ ...value, scope: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Name</Label>
        <Input
          value={value.name}
          onChange={(e) => onChange({ ...value, name: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Description</Label>
        <Textarea
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Prompt Body</Label>
        <Textarea
          className="font-mono text-xs"
          rows={10}
          value={value.prompt_body}
          onChange={(e) => onChange({ ...value, prompt_body: e.target.value })}
          placeholder="Use {{.name}} to reference input slots"
        />
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>Inputs</Label>
          <Button type="button" variant="outline" size="sm" onClick={addSlot}>
            Add input
          </Button>
        </div>
        <div className="flex flex-col gap-2">
          {value.inputs.length === 0 ? (
            <p className="text-xs text-muted-foreground">No input slots.</p>
          ) : null}
          {value.inputs.map((slot, i) => (
            <div key={i} className="flex items-end gap-2">
              <div className="grid flex-1 gap-1">
                <Label className="text-xs">Name</Label>
                <Input
                  value={slot.name}
                  onChange={(e) => updateSlot(i, { name: e.target.value })}
                />
              </div>
              <div className="grid w-40 gap-1">
                <Label className="text-xs">Resource Type</Label>
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
              <div className="flex items-center gap-2 pb-2">
                <Checkbox
                  id={`slot-required-${i}`}
                  checked={slot.required}
                  onCheckedChange={(v) => updateSlot(i, { required: v === true })}
                />
                <Label htmlFor={`slot-required-${i}`} className="text-xs">
                  Required
                </Label>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="remove input"
                onClick={() => removeSlot(i)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-2">
        <Label>Output Asset Kind</Label>
        <Select
          value={value.output_asset_kind}
          onValueChange={(v) => onChange({ ...value, output_asset_kind: v })}
        >
          <SelectTrigger>
            <SelectValue placeholder="Select output kind" />
          </SelectTrigger>
          <SelectContent>
            {(assetKinds ?? []).map((k) => (
              <SelectItem key={k.name} value={k.name}>
                {k.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label>Platform</Label>
        <Input
          value={value.platform}
          onChange={(e) => onChange({ ...value, platform: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Gen Way</Label>
        <Input
          value={value.gen_way}
          onChange={(e) => onChange({ ...value, gen_way: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Gen Type</Label>
        <Input
          value={value.gen_type ?? ""}
          onChange={(e) => onChange({ ...value, gen_type: e.target.value })}
        />
      </div>
      <div className="flex items-center gap-2">
        <Checkbox
          id="needs-confirm"
          checked={value.needs_confirm ?? false}
          onCheckedChange={(v) => onChange({ ...value, needs_confirm: v === true })}
        />
        <Label htmlFor="needs-confirm">Needs Confirm</Label>
      </div>
      <div className="grid gap-2">
        <Label>Candidate Count</Label>
        <Input
          type="number"
          value={String(value.candidate_count ?? 0)}
          onChange={(e) =>
            onChange({ ...value, candidate_count: Number(e.target.value) })
          }
        />
      </div>
      <div className="grid gap-2">
        <Label>Visibility</Label>
        <Select
          value={value.visibility ?? "private"}
          onValueChange={(v) => onChange({ ...value, visibility: v as Visibility })}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {VISIBILITY_OPTIONS.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex gap-2">
        <Button onClick={onSubmit} disabled={isPending}>
          {submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
