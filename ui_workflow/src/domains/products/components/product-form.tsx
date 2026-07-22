import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ProductInput } from "@/domains/products/types";

function stringifyMetadata(metadata: Record<string, unknown> | undefined): string {
  if (!metadata) return "";
  try {
    return JSON.stringify(metadata, null, 2);
  } catch {
    return "";
  }
}

/**
 * Shared form for create/edit. Caller owns state + submit mutation.
 *
 * metadata (jsonb) is edited as JSON in a textarea; the local text buffer is
 * initialised once from `value.metadata` (the parent sets it before mount -
 * see ProductEdit's `value === null` gate). On each keystroke we re-parse:
 * valid JSON updates `value.metadata`; invalid JSON leaves `value.metadata`
 * untouched and surfaces an inline error so the user knows submit will send
 * the last valid shape.
 */
export function ProductForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
}: {
  value: ProductInput;
  onChange: (v: ProductInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
}) {
  const [metadataText, setMetadataText] = useState(() => stringifyMetadata(value.metadata));
  const [metadataError, setMetadataError] = useState<string | null>(null);

  function onMetadataChange(text: string) {
    setMetadataText(text);
    const trimmed = text.trim();
    if (!trimmed) {
      onChange({ ...value, metadata: undefined });
      setMetadataError(null);
      return;
    }
    try {
      const parsed = JSON.parse(trimmed) as Record<string, unknown>;
      onChange({ ...value, metadata: parsed });
      setMetadataError(null);
    } catch (e) {
      setMetadataError(e instanceof Error ? e.message : "Invalid JSON");
    }
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
        <Input value={value.name} onChange={(e) => onChange({ ...value, name: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>Description</Label>
        <Textarea
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Metadata (JSON)</Label>
        <Textarea
          className="font-mono text-xs"
          value={metadataText}
          onChange={(e) => onMetadataChange(e.target.value)}
          rows={8}
          placeholder="{}"
        />
        {metadataError ? (
          <p className="text-xs text-destructive">Invalid JSON: {metadataError}</p>
        ) : null}
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
