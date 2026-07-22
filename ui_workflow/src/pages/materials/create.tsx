import { useState } from "react";
import { useCreateMaterial } from "@/domains/materials/hooks/use-material-mutations";
import { MaterialForm } from "@/domains/materials/components/material-form";
import type { MaterialInput } from "@/domains/materials/types";

export function MaterialsCreate({ onDone }: { onDone: () => void }) {
  const createMaterial = useCreateMaterial();
  const [value, setValue] = useState<MaterialInput>({
    scope: "",
    asset_kind: "",
    name: "",
    url: "",
    visibility: "private",
  });

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Material</h2>
      <MaterialForm
        value={value}
        onChange={setValue}
        onSubmit={() => createMaterial.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createMaterial.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
