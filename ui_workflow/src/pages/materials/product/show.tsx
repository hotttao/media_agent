import {
  Archive,
  ArrowLeft,
  Braces,
  Edit3,
  ExternalLink,
  PackageOpen,
  ShoppingBag,
  Sparkles,
  Trash2,
  Users,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { StatusBadge } from "@/domains/products/components/status-badge";
import { useProduct } from "@/domains/products/hooks/use-product";
import {
  useArchiveProduct,
  useDeleteProduct,
} from "@/domains/products/hooks/use-product-mutations";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
  StructuredDetails,
} from "@/foundation/components/detail-layout";
import { LibraryGallery } from "@/foundation/components/library-gallery";
import { hostShim } from "@/foundation/lib/host-shim";

function parseValue<T>(value: T | string | undefined, fallback: T): T {
  if (!value) return fallback;
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
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
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!product)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这个产品。
      </div>
    );
  const tags = parseValue<string[]>(product.tags, []);
  const actions = parseValue<unknown[]>(product.display_actions, []);
  const metadata = parseValue<Record<string, unknown>>(product.metadata, {});
  const commerce = parseValue<Record<string, unknown>>(
    product.external_commerce,
    {},
  );
  const images = [...(product.images ?? [])].sort(
    (a, b) =>
      Number(b.is_main) - Number(a.is_main) ||
      (a.sort_order ?? 0) - (b.sort_order ?? 0),
  );
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          返回产品列表
        </Button>
        <div className="flex gap-2">
          {product.status !== "archived" ? (
            <Button
              variant="outline"
              size="sm"
              disabled={archive.isPending}
              onClick={() => archive.mutate(id)}
            >
              <Archive className="mr-1.5 h-4 w-4" />
              归档
            </Button>
          ) : null}
          <Button size="sm" onClick={onEdit}>
            <Edit3 className="mr-1.5 h-4 w-4" />
            编辑产品
          </Button>
        </div>
      </div>
      <section className="grid gap-6 lg:grid-cols-[minmax(320px,500px)_1fr] lg:items-start">
        <LibraryGallery
          organizationId={product.organization_id ?? organization.id}
          items={images.map((image, index) => ({
            objectPath: image.object_path,
            label: image.is_main ? "产品主图" : `详情图 ${index + 1}`,
            badge: image.is_main ? "Main image" : undefined,
          }))}
          alt={product.name}
        />
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={product.status} />
            <span className="rounded-full border border-border/70 bg-muted/50 px-2.5 py-1 text-xs">
              {product.scope}
            </span>
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Product
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {product.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {product.description || "这个产品还没有填写描述。"}
            </p>
          </div>
          {tags.length ? (
            <div className="flex flex-wrap gap-2">
              {tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded-full border border-primary/15 bg-primary/5 px-3 py-1 text-xs font-medium text-primary"
                >
                  {tag}
                </span>
              ))}
            </div>
          ) : null}
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label="创建时间">
              {formatDetailDate(product.created_at)}
            </DetailField>
            <DetailField label="最近更新">
              {formatDetailDate(product.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <div className="grid gap-5 lg:grid-cols-2">
        <DetailSection
          title="目标人群"
          description="产品内容主要面向的用户画像"
          icon={Users}
        >
          <p className="whitespace-pre-wrap text-sm leading-7">
            {product.target_audience || "暂未填写目标人群。"}
          </p>
        </DetailSection>
        <DetailSection
          title="产品详情"
          description="卖点、规格和内容生产所需信息"
          icon={PackageOpen}
        >
          <p className="whitespace-pre-wrap text-sm leading-7">
            {product.product_details || "暂未填写产品详情。"}
          </p>
        </DetailSection>
      </div>
      {actions.length ? (
        <DetailSection
          title="展示动作"
          description="拍摄或生成内容时建议采用的展示方式"
          icon={Sparkles}
        >
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {actions.map((action, index) => (
              <div
                key={`${String(action)}-${index}`}
                className="flex items-center gap-2 rounded-xl border border-border/60 bg-muted/25 px-3.5 py-3 text-sm"
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                  {index + 1}
                </span>
                <span>{String(action)}</span>
              </div>
            ))}
          </div>
        </DetailSection>
      ) : null}
      <div className="grid gap-5 lg:grid-cols-2">
        {Object.keys(commerce).length ? (
          <DetailSection
            title="商城信息"
            description="与外部商品页面和交易渠道关联的信息"
            icon={ExternalLink}
          >
            <StructuredDetails value={commerce} />
          </DetailSection>
        ) : null}
        {Object.keys(metadata).length ? (
          <DetailSection
            title="扩展属性"
            description="尚未进入稳定产品字段的补充信息"
            icon={Braces}
          >
            <StructuredDetails value={metadata} />
          </DetailSection>
        ) : null}
      </div>
      <DetailSection
        title="审计信息"
        description="产品记录的创建与最后维护信息"
        icon={ShoppingBag}
      >
        <DetailFieldGrid>
          <DetailField label="创建人">{product.created_by}</DetailField>
          <DetailField label="更新人">{product.updated_by}</DetailField>
          <DetailField label="创建时间">
            {formatDetailDate(product.created_at)}
          </DetailField>
          <DetailField label="更新时间">
            {formatDetailDate(product.updated_at)}
          </DetailField>
        </DetailFieldGrid>
      </DetailSection>
      <DetailSection
        title="产品管理"
        description="删除后产品将不能再被新的工作流使用"
        icon={ShoppingBag}
        className="border-destructive/20"
      >
        <Button
          variant="destructive"
          size="sm"
          disabled={remove.isPending}
          onClick={() => remove.mutate(id, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          删除产品
        </Button>
      </DetailSection>
    </DetailPage>
  );
}
