import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function MaterialCreatePage() {
  return (
    <ResourcePageScaffold
      resourceKey="material"
      mode="create"
      listPath="/materials"
    />
  );
}
