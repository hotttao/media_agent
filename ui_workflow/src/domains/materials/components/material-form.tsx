import { useAssetKinds } from "@/domains/materials/hooks/use-asset-kinds";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { MaterialInput, Visibility } from "@/domains/materials/types";

/** Shared form for create/edit. Caller owns state + submit mutation. */
export function MaterialForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
}: {
  value: MaterialInput;
  onChange: (v: MaterialInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
}) {
  const { data: kinds } = useAssetKinds("MATERIAL");
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
        <Label>Asset Kind</Label>
        <Select
          value={value.asset_kind}
          onValueChange={(v) => onChange({ ...value, asset_kind: v })}
        >
          <SelectTrigger>
            <SelectValue placeholder="Select kind" />
          </SelectTrigger>
          <SelectContent>
            {(kinds ?? []).map((k) => (
              <SelectItem key={k.name} value={k.name}>
                {k.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label>Name</Label>
        <Input value={value.name} onChange={(e) => onChange({ ...value, name: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>URL</Label>
        <Input value={value.url} onChange={(e) => onChange({ ...value, url: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>Content</Label>
        <Textarea
          value={value.content ?? ""}
          onChange={(e) => onChange({ ...value, content: e.target.value })}
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
            <SelectItem value="private">private</SelectItem>
            <SelectItem value="team">team</SelectItem>
            <SelectItem value="public">public</SelectItem>
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
