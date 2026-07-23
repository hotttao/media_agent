import { useState } from "react";
import {
  useCreateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import type { ValidationResult, WorkflowInput } from "@/domains/workflows/types";

export function WorkflowCreate({ onDone }: { onDone: () => void }) {
  const createWorkflow = useCreateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = useState<WorkflowInput>({
    scope: "",
    name: "",
    output_step_key: "",
    steps: [],
    visibility: "private",
  });
  const [validationResult, setValidationResult] = useState<ValidationResult | null>(null);

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Workflow</h2>
      <WorkflowForm
        value={value}
        onChange={setValue}
        onSubmit={() => createWorkflow.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createWorkflow.isPending}
        onCancel={onDone}
        onValidate={() => validate.mutate(value, { onSuccess: setValidationResult })}
        validationResult={validationResult}
      />
    </div>
  );
}
