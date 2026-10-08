import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@/foundation/lib/api-client";
import { hostShim } from "@/foundation/lib/host-shim";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { getTemplateVersion } from "@/domains/templates/lib/api";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import {
  useCreateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import type {
  ValidationResult,
  WorkflowInput,
} from "@/domains/workflows/types";

export function WorkflowCreate({
  onDone,
  instanceId,
}: {
  onDone: (id?: string) => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const createWorkflow = useCreateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = hostShim.useInstanceState<WorkflowInput>(
    instanceId,
    "workflow_create_draft",
    () => ({
      scope: "",
      name: "",
      output_step_key: "",
      steps: [],
      visibility: "private",
    }),
  );
  const workspaces = useQuery({
    queryKey: ["available-workspaces", shell.organization.id],
    enabled: Boolean(shell.organization.id) && !shell.isLoading,
    queryFn: () =>
      api.request<{
        workspaces?: { id: string; name: string; organization_id: string }[];
      }>("/workspaces"),
  });
  const choices = (workspaces.data?.workspaces ?? []).filter(
    (workspace) => workspace.organization_id === shell.organization.id,
  );
  const [validationResult, setValidationResult] = useState<{
    fingerprint: string;
    result: ValidationResult;
  } | null>(null);
  const { data: templates } = useTemplatesList({
    scope: value.scope || undefined,
    page_size: 100,
  });

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      <WorkflowForm
        settingsExtra={
          <div className="flex flex-wrap items-center gap-3 sm:col-span-2">
            <label htmlFor="create-workflow-workspace">
              {t("editorContext.workspace")}
            </label>
            <select
              id="create-workflow-workspace"
              className="h-9 rounded-lg border bg-background px-3"
              value={value.workspace_id ?? ""}
              onChange={(event) =>
                setValue({ ...value, workspace_id: event.target.value })
              }
              disabled={workspaces.isLoading || workspaces.isError}
            >
              <option value="">{t("editorContext.chooseWorkspace")}</option>
              {choices.map((workspace) => (
                <option key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </option>
              ))}
            </select>
            {workspaces.isError ? (
              <p role="alert" className="text-destructive">
                {workspaces.error.message}
              </p>
            ) : null}
          </div>
        }
        resolveTemplate={async (option) => ({
          ...(await getTemplateVersion(option.id, option.version)),
          name: option.name,
        })}
        value={value}
        onChange={setValue}
        onSubmit={() => {
          if (
            value.workspace_id &&
            choices.some((workspace) => workspace.id === value.workspace_id)
          )
            createWorkflow.mutate(
              { ...value, organization_id: shell.organization.id },
              {
                onSuccess: (receipt) => {
                  setValue({
                    scope: "",
                    name: "",
                    output_step_key: "",
                    steps: [],
                    visibility: "private",
                  });
                  onDone(receipt.id);
                },
              },
            );
        }}
        submitLabel={t("editor.savedDraft")}
        isPending={createWorkflow.isPending}
        onCancel={onDone}
        onValidate={() =>
          validate.mutate(value, {
            onSuccess: (result) =>
              setValidationResult({
                fingerprint: JSON.stringify(value),
                result,
              }),
          })
        }
        validationResult={
          validationResult?.fingerprint === JSON.stringify(value)
            ? validationResult.result
            : null
        }
        templateOptions={templates?.items}
        submitDisabled={
          !value.workspace_id ||
          !choices.some((workspace) => workspace.id === value.workspace_id)
        }
        error={createWorkflow.error?.message || validate.error?.message}
      />
    </div>
  );
}
