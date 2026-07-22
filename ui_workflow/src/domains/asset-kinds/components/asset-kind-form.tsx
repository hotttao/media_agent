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
import type {
  AssetKindInput,
  AssetKindMediaType,
  AssetKindResourceType,
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
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-2">
        <Label>Name</Label>
        <Input
          value={value.name}
          disabled={nameDisabled}
          onChange={(e) => onChange({ ...value, name: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Resource Type</Label>
        <Select
          value={value.resource_type}
          onValueChange={(v) =>
            onChange({ ...value, resource_type: v as AssetKindResourceType })
          }
        >
          <SelectTrigger>
            <SelectValue placeholder="Select resource type" />
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
        <Label>Media Type</Label>
        <Select
          value={value.media_type}
          onValueChange={(v) =>
            onChange({ ...value, media_type: v as AssetKindMediaType })
          }
        >
          <SelectTrigger>
            <SelectValue placeholder="Select media type" />
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
        <Label>Description</Label>
        <Textarea
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
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
