import { useState } from "react";
import { useCreateProduct } from "@/domains/products/hooks/use-product-mutations";
import { ProductForm } from "@/domains/products/components/product-form";
import type { ProductInput } from "@/domains/products/types";

export function ProductCreate({ onDone }: { onDone: () => void }) {
  const createProduct = useCreateProduct();
  const [value, setValue] = useState<ProductInput>({
    scope: "",
    name: "",
    description: "",
    metadata: undefined,
  });

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Product</h2>
      <ProductForm
        value={value}
        onChange={setValue}
        onSubmit={() => createProduct.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createProduct.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
