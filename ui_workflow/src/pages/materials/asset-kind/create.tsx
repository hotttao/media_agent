import { useState } from "react";
import { useCreateAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import { AssetKindForm } from "@/domains/asset-kinds/components/asset-kind-form";
import type { AssetKindInput } from "@/domains/asset-kinds/types";

export function AssetKindCreate({ onDone }: { onDone: () => void }) {
  const createAssetKind = useCreateAssetKind();
  const [value, setValue] = useState<AssetKindInput>({
    name: "",
    resource_type: "MATERIAL",
    media_type: "IMAGE",
  });

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Asset Kind</h2>
      <AssetKindForm
        value={value}
        onChange={setValue}
        onSubmit={() => createAssetKind.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createAssetKind.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
