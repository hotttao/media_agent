import { useState } from "react";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import {
  useCreateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import type {
  ValidationResult,
  WorkflowInput,
} from "@/domains/workflows/types";

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
  const [validationResult, setValidationResult] =
    useState<ValidationResult | null>(null);
  const { data: templates } = useTemplatesList({
    scope: value.scope || undefined,
    page_size: 100,
  });

  return (
    <div className="h-full overflow-y-auto bg-gradient-to-b from-slate-50/80 to-background dark:from-slate-950/80">
      <div className="mx-auto flex min-h-full max-w-[1600px] flex-col gap-5 p-4 sm:p-6 lg:p-8">
        <div>
          <div className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            Workflow Studio
          </div>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight">
            新建工作流
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            用节点和输入绑定定义业务流程，发布时再编译为 Conductor Workflow。
          </p>
        </div>
        <WorkflowForm
          value={value}
          onChange={setValue}
          onSubmit={() => createWorkflow.mutate(value, { onSuccess: onDone })}
          submitLabel="Create"
          isPending={createWorkflow.isPending}
          onCancel={onDone}
          onValidate={() =>
            validate.mutate(value, { onSuccess: setValidationResult })
          }
          validationResult={validationResult}
          templateOptions={templates?.items}
        />
      </div>
    </div>
  );
}
