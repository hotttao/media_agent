import { Archive, ArrowLeft, Edit3, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { StatusBadge } from "@/domains/products/components/status-badge";
import { useProduct } from "@/domains/products/hooks/use-product";
import {
  useArchiveProduct,
  useDeleteProduct,
} from "@/domains/products/hooks/use-product-mutations";
import { LibraryImage } from "@/foundation/components/library-image";
import { hostShim } from "@/foundation/lib/host-shim";

function parseJSON<T>(value: T | string | undefined, fallback: T): T {
  if (!value) return fallback;
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
function Info({
  label,
  children,
}: {
  label: string;
  children?: React.ReactNode;
}) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-1 whitespace-pre-wrap text-sm">{children || "-"}</div>
    </div>
  );
}

export function ProductShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: product, isLoading } = useProduct(id);
  const remove = useDeleteProduct();
  const archive = useArchiveProduct();
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!product)
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  const tags = parseJSON<string[]>(product.tags, []);
  const actions = parseJSON<unknown[]>(product.display_actions, []);
  const metadata = parseJSON<Record<string, unknown>>(product.metadata, {});
  const commerce = parseJSON<Record<string, unknown>>(
    product.external_commerce,
    {},
  );
  const images = [...(product.images ?? [])].sort(
    (a, b) =>
      Number(b.is_main) - Number(a.is_main) ||
      (a.sort_order ?? 0) - (b.sort_order ?? 0),
  );
  return (
    <div className="flex h-full flex-col gap-6 overflow-auto p-5">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1 h-4 w-4" />
          返回
        </Button>
        <div className="flex gap-2">
          {product.status !== "archived" ? (
            <Button
              variant="outline"
              size="sm"
              disabled={archive.isPending}
              onClick={() => archive.mutate(id)}
            >
              <Archive className="mr-1 h-4 w-4" />
              归档
            </Button>
          ) : null}
          <Button size="sm" onClick={onEdit}>
            <Edit3 className="mr-1 h-4 w-4" />
            编辑
          </Button>
        </div>
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(300px,480px)_1fr]">
        <div className="grid grid-cols-2 gap-2">
          {images.length ? (
            images.map((image, index) => (
              <div
                key={image.id ?? image.object_path}
                className={`overflow-hidden rounded-xl border bg-muted/20 ${index === 0 ? "col-span-2 aspect-[4/3]" : "aspect-square"}`}
              >
                <LibraryImage
                  organizationId={product.organization_id ?? organization.id}
                  objectPath={image.object_path}
                  alt={`${product.name} ${index + 1}`}
                  className="object-contain"
                />
              </div>
            ))
          ) : (
            <div className="col-span-2 aspect-[4/3] overflow-hidden rounded-xl border">
              <LibraryImage
                organizationId={organization.id}
                alt={product.name}
              />
            </div>
          )}
        </div>
        <div className="grid content-start gap-5">
          <div className="flex items-center gap-2">
            <StatusBadge status={product.status} />
            <span className="rounded-full bg-muted px-2 py-1 text-xs">
              {product.scope}
            </span>
          </div>
          <div>
            <h2 className="text-2xl font-semibold">{product.name}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {product.description || "暂无描述"}
            </p>
          </div>
          <Info label="目标人群">{product.target_audience}</Info>
          <Info label="产品详情">{product.product_details}</Info>
          {tags.length ? (
            <div className="flex flex-wrap gap-2">
              {tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded-full border px-2.5 py-1 text-xs"
                >
                  {tag}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      </div>
      {actions.length ? (
        <section className="rounded-xl border p-4">
          <h3 className="mb-3 font-medium">展示动作</h3>
          <div className="flex flex-wrap gap-2">
            {actions.map((action, index) => (
              <span
                key={`${String(action)}-${index}`}
                className="rounded-lg bg-muted px-3 py-2 text-sm"
              >
                {String(action)}
              </span>
            ))}
          </div>
        </section>
      ) : null}
      <div className="grid gap-4 md:grid-cols-2">
        {Object.keys(commerce).length ? (
          <section className="rounded-xl border p-4">
            <h3 className="mb-2 font-medium">外部商城信息</h3>
            <pre className="overflow-auto rounded bg-muted p-3 text-xs">
              {JSON.stringify(commerce, null, 2)}
            </pre>
          </section>
        ) : null}
        {Object.keys(metadata).length ? (
          <section className="rounded-xl border p-4">
            <h3 className="mb-2 font-medium">Metadata</h3>
            <pre className="overflow-auto rounded bg-muted p-3 text-xs">
              {JSON.stringify(metadata, null, 2)}
            </pre>
          </section>
        ) : null}
      </div>
      <dl className="grid grid-cols-[100px_1fr] gap-2 text-sm">
        <dt className="text-muted-foreground">Created By</dt>
        <dd>{product.created_by}</dd>
        <dt className="text-muted-foreground">Updated By</dt>
        <dd>{product.updated_by || "-"}</dd>
        <dt className="text-muted-foreground">Created At</dt>
        <dd>{String(product.created_at)}</dd>
        <dt className="text-muted-foreground">Updated At</dt>
        <dd>{String(product.updated_at)}</dd>
      </dl>
      <Button
        className="self-start"
        variant="destructive"
        size="sm"
        onClick={() => remove.mutate(id, { onSuccess: onBack })}
      >
        <Trash2 className="mr-1 h-4 w-4" />
        删除产品
      </Button>
    </div>
  );
}
