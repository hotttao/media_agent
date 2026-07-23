import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ValidationResult, Visibility, WorkflowInput } from "@/domains/workflows/types";

/** Shared form for create/edit. `steps` is edited as JSON for now - the DAG
 * visual editor (drag/drop nodes + bindings) is a follow-up (TODO). */
export function WorkflowForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  onValidate,
  validationResult,
}: {
  value: WorkflowInput;
  onChange: (v: WorkflowInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  onValidate?: () => void;
  validationResult?: ValidationResult | null;
}) {
  const stepsJson = JSON.stringify(value.steps ?? [], null, 2);
  function setSteps(json: string) {
    try {
      onChange({ ...value, steps: JSON.parse(json) as WorkflowInput["steps"] });
    } catch {
      // invalid JSON - keep editing without updating steps
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-2">
        <Label>Scope</Label>
        <Input value={value.scope} onChange={(e) => onChange({ ...value, scope: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>Name</Label>
        <Input value={value.name} onChange={(e) => onChange({ ...value, name: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>Description</Label>
        <Input
          value={value.description ?? ""}
          onChange={(e) => onChange({ ...value, description: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Output Step Key</Label>
        <Input
          value={value.output_step_key}
          onChange={(e) => onChange({ ...value, output_step_key: e.target.value })}
        />
      </div>
      <div className="grid gap-2">
        <Label>Steps (JSON - DAG visual editor TODO)</Label>
        <Textarea
          className="font-mono text-xs"
          rows={12}
          value={stepsJson}
          onChange={(e) => setSteps(e.target.value)}
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
      {onValidate && (
        <Button variant="outline" onClick={onValidate} disabled={isPending}>
          Validate
        </Button>
      )}
      {validationResult && (
        <div className="text-xs">
          {validationResult.errors.length === 0 ? (
            <span className="text-success">Valid ✓</span>
          ) : (
            <ul className="text-destructive">
              {validationResult.errors.map((e, i) => (
                <li key={i}>
                  {e.step_key ? `[${e.step_key}] ` : ""}
                  {e.message}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
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
