import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function MaterialShowPage() {
  return (
    <ResourcePageScaffold
      resourceKey="material"
      mode="show"
      listPath="/materials"
    />
  );
}
