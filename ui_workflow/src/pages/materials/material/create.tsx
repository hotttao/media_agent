import { useState } from "react";
import { MaterialForm } from "@/domains/materials/components/material-form";
import { useCreateMaterial } from "@/domains/materials/hooks/use-material-mutations";
import type { MaterialInput } from "@/domains/materials/types";

export function MaterialsCreate({ onDone }: { onDone: () => void }) {
  const createMaterial = useCreateMaterial();
  const [value, setValue] = useState<MaterialInput>({
    scope: "",
    asset_kind: "",
    name: "",
    object_path: "",
    visibility: "private",
  });

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
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
