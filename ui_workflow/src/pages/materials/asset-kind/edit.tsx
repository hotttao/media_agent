import { useEffect, useState } from "react";
import { useAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind";
import { useUpdateAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import { AssetKindForm } from "@/domains/asset-kinds/components/asset-kind-form";
import { Spinner } from "@/components/ui/spinner";
import type { AssetKindInput } from "@/domains/asset-kinds/types";

export function AssetKindEdit({ name, onDone }: { name: string; onDone: () => void }) {
  const { data: assetKind, isLoading } = useAssetKind(name);
  const updateAssetKind = useUpdateAssetKind();
  const [value, setValue] = useState<AssetKindInput | null>(null);

  useEffect(() => {
    if (assetKind) {
      setValue({
        name: assetKind.name,
        resource_type: assetKind.resource_type,
        media_type: assetKind.media_type,
        description: assetKind.description,
      });
    }
  }, [assetKind]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">Edit Asset Kind</h2>
      <AssetKindForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateAssetKind.mutate({ name, patch: value }, { onSuccess: onDone })
        }
        submitLabel="Save"
        isPending={updateAssetKind.isPending}
        onCancel={onDone}
        nameDisabled
      />
    </div>
  );
}
