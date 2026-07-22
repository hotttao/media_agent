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
import type { VirtualIpInput, Visibility } from "@/domains/virtual-ips/types";

/** Shared form for create/edit. Caller owns state + submit mutation.
 * Scope is set by the caller (not in the form) - see create/edit pages. */
export function VirtualIpForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
}: {
  value: VirtualIpInput;
  onChange: (v: VirtualIpInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
}) {
  return (
    <div className="flex flex-col gap-4">
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
        <Label>Avatar URL</Label>
        <Input
          value={value.avatar_url ?? ""}
          onChange={(e) => onChange({ ...value, avatar_url: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Full Body URL</Label>
        <Input
          value={value.full_body_url ?? ""}
          onChange={(e) =>
            onChange({ ...value, full_body_url: e.target.value })
          }
        />
      </div>
      <div className="grid gap-2">
        <Label>Three View URL</Label>
        <Input
          value={value.three_view_url ?? ""}
          onChange={(e) =>
            onChange({ ...value, three_view_url: e.target.value })
          }
        />
      </div>
      <div className="grid gap-2">
        <Label>Nine View URL</Label>
        <Input
          value={value.nine_view_url ?? ""}
          onChange={(e) =>
            onChange({ ...value, nine_view_url: e.target.value })
          }
        />
      </div>
      <div className="grid gap-2">
        <Label>Visibility</Label>
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
