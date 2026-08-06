import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function ProductShowPage() {
  return (
    <ResourcePageScaffold
      resourceKey="product"
      mode="show"
      listPath="/products"
    />
  );
}
