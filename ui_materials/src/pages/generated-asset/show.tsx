import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function GeneratedAssetShowPage() {
  return (
    <ResourcePageScaffold
      resourceKey="generatedAsset"
      mode="show"
      listPath="/generated-assets"
    />
  );
}
