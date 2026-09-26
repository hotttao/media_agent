import { useEffect, useState } from "react";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import {
  useChangeTemplateVisibility,
  useUpdateTemplate,
} from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import { Spinner } from "@/components/ui/spinner";
import type { TemplateInput } from "@/domains/templates/types";

export function TemplateEdit({ id, onDone }: { id: string; onDone: () => void }) {
  const { data: template, isLoading } = useTemplate(id);
  const updateTemplate = useUpdateTemplate();
  const changeVisibility = useChangeTemplateVisibility();
  const [value, setValue] = useState<TemplateInput | null>(null);

  useEffect(() => {
    if (template) {
      setValue({
        scope: template.scope,
        name: template.name,
        description: template.description,
        prompt_body: template.prompt_body,
        inputs: template.inputs.map((s) => ({
          name: s.name,
          asset_kind: s.asset_kind,
          resource_type: s.resource_type,
          required: s.required,
          display_order: s.display_order,
        })),
        output_asset_kind: template.output_asset_kind,
        platform: template.platform,
        gen_way: template.gen_way,
        gen_type: template.gen_type,
        needs_confirm: template.needs_confirm,
        candidate_count: template.candidate_count,
        capability_key: template.capability_key,
        capability_version: template.capability_version,
        invocation_mapping: template.invocation_mapping,
        visibility: template.visibility,
      });
    }
  }, [template]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  function handleSubmit() {
    if (!value) return;
    const originalVisibility = template?.visibility;
    updateTemplate.mutate(
      { id, patch: value },
      {
        onSuccess: () => {
          // proto's UpdateTemplateReq does not carry visibility; if the user
          // changed it, apply via the dedicated endpoint.
          if (value.visibility && value.visibility !== originalVisibility) {
            changeVisibility.mutate({
              id,
              input: { visibility: value.visibility },
            });
          }
          onDone();
        },
      },
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">Edit Template</h2>
      <TemplateForm
        value={value}
        onChange={setValue}
        onSubmit={handleSubmit}
        submitLabel="Save"
        isPending={updateTemplate.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
