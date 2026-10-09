import { useTranslation } from "react-i18next";
import { useCreateTemplate } from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import type { TemplateInput } from "@/domains/templates/types";
import { hostShim } from "@/foundation/lib/host-shim";

export function TemplateCreate({
  onDone,
  instanceId,
}: {
  onDone: (id?: string) => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const createTemplate = useCreateTemplate();
  const [value, setValue] = hostShim.useInstanceState<TemplateInput>(
    instanceId,
    "workflow_template_create_draft",
    () => ({
      scope: "",
      name: "",
      description: "",
      prompt_body: "",
      inputs: [],
      output_asset_kind: "",
      platform: "",
      gen_way: "",
      gen_type: "",
      needs_confirm: false,
      candidate_count: 0,
      capability_key: "",
      capability_version: 1,
      invocation_mapping: { prompt: { op: "render_template" } },
      visibility: "private",
    }),
  );

  function handleSubmit() {
    if (shell.isLoading || !shell.organization.id) return;
    createTemplate.mutate(
      { ...value, organization_id: shell.organization.id },
      {
        onSuccess: (created) => {
          setValue({
            scope: "",
            name: "",
            prompt_body: "",
            inputs: [],
            output_asset_kind: "",
            platform: "",
            gen_way: "",
            capability_key: "",
            capability_version: 1,
            invocation_mapping: { prompt: { op: "render_template" } },
            visibility: "private",
          });
          onDone(created.id);
        },
      },
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {createTemplate.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {createTemplate.error?.message}
        </p>
      ) : null}
      <TemplateForm
        value={value}
        onChange={setValue}
        onSubmit={handleSubmit}
        submitLabel={t("editor.savedDraft")}
        isPending={
          createTemplate.isPending || shell.isLoading || !shell.organization.id
        }
        onCancel={onDone}
      />
    </div>
  );
}
