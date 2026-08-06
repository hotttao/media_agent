import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function GeneratedAssetListPage() {
  return (
    <ResourcePageScaffold
      resourceKey="generatedAsset"
      mode="list"
      listPath="/generated-assets"
    />
  );
}
