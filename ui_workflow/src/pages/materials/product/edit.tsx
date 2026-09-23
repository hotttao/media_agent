import { useEffect, useState } from "react";
import { Spinner } from "@/components/ui/spinner";
import { ProductForm } from "@/domains/products/components/product-form";
import { useProduct } from "@/domains/products/hooks/use-product";
import { useUpdateProduct } from "@/domains/products/hooks/use-product-mutations";
import type { ProductInput } from "@/domains/products/types";

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

export function ProductEdit({
  id,
  onDone,
}: {
  id: string;
  onDone: () => void;
}) {
  const { data: product, isLoading } = useProduct(id);
  const updateProduct = useUpdateProduct();
  const [value, setValue] = useState<ProductInput | null>(null);

  useEffect(() => {
    if (product) {
      setValue({
        scope: product.scope,
        name: product.name,
        description: product.description,
        metadata: editableMetadata(product.metadata),
      });
    }
  }, [product]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">Edit Product</h2>
      <ProductForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateProduct.mutate({ id, patch: value }, { onSuccess: onDone })
        }
        submitLabel="Save"
        isPending={updateProduct.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
