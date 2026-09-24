import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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

function editableList(
  value: unknown[] | string | undefined,
): unknown[] | undefined {
  if (Array.isArray(value)) return value;
  if (!value) return undefined;
  try {
    const parsed = JSON.parse(value) as unknown;
    return Array.isArray(parsed) ? parsed : undefined;
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
  const { t } = useTranslation();
  const { data: product, isLoading } = useProduct(id);
  const updateProduct = useUpdateProduct();
  const [value, setValue] = useState<ProductInput | null>(null);

  useEffect(() => {
    if (product) {
      setValue({
        scope: product.scope,
        name: product.name,
        description: product.description,
        target_audience: product.target_audience,
        product_details: product.product_details,
        display_actions: editableList(product.display_actions),
        tags: editableList(product.tags)?.map(String),
        external_commerce: editableMetadata(product.external_commerce),
        metadata: editableMetadata(product.metadata),
        images: product.images?.map(
          ({ object_path, is_main = false, sort_order = 0 }) => ({
            object_path,
            is_main,
            sort_order,
          }),
        ),
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
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <h2 className="text-lg font-semibold">{t("productForm.editTitle")}</h2>
      <ProductForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateProduct.mutate({ id, patch: value }, { onSuccess: onDone })
        }
        submitLabel={t("common.save")}
        isPending={updateProduct.isPending}
        onCancel={onDone}
        isEdit
      />
    </div>
  );
}
