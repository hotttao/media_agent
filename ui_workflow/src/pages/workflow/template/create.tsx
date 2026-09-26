import { useState } from "react";
import { useCreateTemplate } from "@/domains/templates/hooks/use-template-mutations";
import { useChangeTemplateVisibility } from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import { Spinner } from "@/components/ui/spinner";
import type { TemplateInput } from "@/domains/templates/types";

export function TemplateCreate({ onDone }: { onDone: () => void }) {
  const createTemplate = useCreateTemplate();
  const changeVisibility = useChangeTemplateVisibility();
  const [value, setValue] = useState<TemplateInput>({
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
    capability_key: "media.image.model",
    capability_version: 1,
    invocation_mapping: { prompt: { op: "render_template" } },
    visibility: "private",
  });

  function handleSubmit() {
    createTemplate.mutate(value, {
      onSuccess: (created) => {
        // proto's CreateTemplateReq does not carry visibility; if the user
        // selected a non-default value, apply it via the dedicated endpoint.
        if (value.visibility && value.visibility !== "private") {
          changeVisibility.mutate({
            id: created.id,
            input: { visibility: value.visibility },
          });
        }
        onDone();
      },
    });
  }

  if (createTemplate.isPending) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Template</h2>
      <TemplateForm
        value={value}
        onChange={setValue}
        onSubmit={handleSubmit}
        submitLabel="Create"
        isPending={createTemplate.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
