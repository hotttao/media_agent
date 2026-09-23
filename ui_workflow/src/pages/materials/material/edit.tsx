import { useEffect, useState } from "react";
import { Spinner } from "@/components/ui/spinner";
import { MaterialForm } from "@/domains/materials/components/material-form";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { useUpdateMaterial } from "@/domains/materials/hooks/use-material-mutations";
import type { MaterialInput } from "@/domains/materials/types";

function editableMetadata(
  metadata: Record<string, unknown> | string | undefined,
): Record<string, unknown> | undefined {
  if (typeof metadata !== "string") return metadata;
  try {
    return JSON.parse(metadata) as Record<string, unknown>;
  } catch {
    return undefined;
  }
}

export function MaterialsEdit({
  id,
  onDone,
}: {
  id: string;
  onDone: () => void;
}) {
  const { data: material, isLoading } = useMaterial(id);
  const updateMaterial = useUpdateMaterial();
  const [value, setValue] = useState<MaterialInput | null>(null);

  useEffect(() => {
    if (material) {
      setValue({
        scope: material.scope,
        asset_kind: material.asset_kind,
        name: material.name,
        url: material.url ?? material.object_path ?? "",
        content: material.content,
        visibility: material.visibility,
        team_id: material.team_id,
        metadata: editableMetadata(material.metadata),
      });
    }
  }, [material]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">Edit Material</h2>
      <MaterialForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateMaterial.mutate({ id, patch: value }, { onSuccess: onDone })
        }
        submitLabel="Save"
        isPending={updateMaterial.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
