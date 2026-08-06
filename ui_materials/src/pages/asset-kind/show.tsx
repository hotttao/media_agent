import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function AssetKindShowPage() {
  return (
    <ResourcePageScaffold
      resourceKey="assetKind"
      mode="show"
      listPath="/settings/asset-kinds"
    />
  );
}
