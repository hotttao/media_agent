import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ProductForm } from "@/domains/products/components/product-form";
import { useCreateProduct } from "@/domains/products/hooks/use-product-mutations";
import type { ProductInput } from "@/domains/products/types";

export function ProductCreate({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const createProduct = useCreateProduct();
  const [value, setValue] = useState<ProductInput>({
    scope: "",
    name: "",
    description: "",
    metadata: undefined,
  });

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <h2 className="text-lg font-semibold">{t("productForm.createTitle")}</h2>
      <ProductForm
        value={value}
        onChange={setValue}
        onSubmit={() => createProduct.mutate(value, { onSuccess: onDone })}
        submitLabel={t("common.create")}
        isPending={createProduct.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
