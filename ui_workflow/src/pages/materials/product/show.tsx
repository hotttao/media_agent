import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RelatedGeneratedAssets } from "@/domains/generated-assets/components/related-generated-assets";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import { ListState } from "@/foundation/components/resource-list-layout";
import { ProductWorkflowLaunch } from "./workflow-launch";
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
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const { data: product, isLoading, isError, error, refetch } = useProduct(id);
  const remove = useDeleteProduct();
  const archive = useArchiveProduct();
  const { organization } = hostShim.useShellContext();
  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!product)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {t("productDetail.notFound")}
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
          {t("productDetail.back")}
        </Button>
        <div className="flex gap-2">
          <ProductWorkflowLaunch
            product={product}
            organizationId={product.organization_id || organization.id}
          />
          {product.status !== "archived" ? (
            <ConfirmActionButton
              variant="outline"
              size="sm"
              disabled={archive.isPending}
              onConfirm={() => archive.mutateAsync(id)}
            >
              <Archive className="mr-1.5 h-4 w-4" />
              {t("productDetail.archive")}
            </ConfirmActionButton>
          ) : null}
          <Button size="sm" onClick={onEdit}>
            <Edit3 className="mr-1.5 h-4 w-4" />
            {t("productDetail.edit")}
          </Button>
        </div>
      </div>
      <section className="media-detail-grid items-start">
        <LibraryGallery
          organizationId={product.organization_id ?? organization.id}
          items={images.map((image, index) => ({
            objectPath: image.object_path,
            label: image.is_main
              ? t("productDetail.mainImage")
              : t("productDetail.detailImage", { index: index + 1 }),
            badge: image.is_main ? t("productDetail.mainImage") : undefined,
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
              {t("productDetail.type")}
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {product.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {product.description || t("productDetail.noDescription")}
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
            <DetailField label={t("materialDetail.createdAt")}>
              {formatDetailDate(product.created_at)}
            </DetailField>
            <DetailField label={t("materialDetail.updatedAt")}>
              {formatDetailDate(product.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <Tabs
        key={`${product.organization_id || organization.id}:${id}`}
        defaultValue="info"
        className="min-w-0"
      >
        <TabsList
          aria-label={t("productDetail.sections")}
          className="mb-4 w-full justify-start sm:w-auto"
        >
          <TabsTrigger value="info" className="flex-1">
            {t("productDetail.infoTab")}
          </TabsTrigger>
          <TabsTrigger value="related" className="flex-1">
            {t("productDetail.relatedTab")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="info" className="mt-0 space-y-5">
          <div className="grid gap-5 lg:grid-cols-2">
            <DetailSection
              title={t("productForm.audience")}
              description={t("productDetail.audienceHint")}
              icon={Users}
            >
              <p className="whitespace-pre-wrap text-sm leading-7">
                {product.target_audience || t("productDetail.noAudience")}
              </p>
            </DetailSection>
            <DetailSection
              title={t("productForm.details")}
              description={t("productDetail.detailsHint")}
              icon={PackageOpen}
            >
              <p className="whitespace-pre-wrap text-sm leading-7">
                {product.product_details || t("productDetail.noDetails")}
              </p>
            </DetailSection>
          </div>
          {actions.length ? (
            <DetailSection
              title={t("productForm.actions")}
              description={t("productDetail.actionsHint")}
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
                title={t("productDetail.commerce")}
                description={t("productDetail.commerceHint")}
                icon={ExternalLink}
              >
                <StructuredDetails value={commerce} />
              </DetailSection>
            ) : null}
            {Object.keys(metadata).length ? (
              <DetailSection
                title={t("materialDetail.metadata")}
                description={t("productDetail.metadataHint")}
                icon={Braces}
              >
                <StructuredDetails value={metadata} />
              </DetailSection>
            ) : null}
          </div>
          <DetailSection
            title={t("productDetail.audit")}
            description={t("productDetail.auditHint")}
            icon={ShoppingBag}
          >
            <DetailFieldGrid>
              <DetailField label={t("productDetail.createdBy")}>
                {product.created_by}
              </DetailField>
              <DetailField label={t("productDetail.updatedBy")}>
                {product.updated_by}
              </DetailField>
              <DetailField label={t("materialDetail.createdAt")}>
                {formatDetailDate(product.created_at)}
              </DetailField>
              <DetailField label={t("materialDetail.updatedAt")}>
                {formatDetailDate(product.updated_at)}
              </DetailField>
            </DetailFieldGrid>
          </DetailSection>
          <DetailSection
            title={t("productDetail.management")}
            description={t("productDetail.managementHint")}
            icon={ShoppingBag}
            className="border-destructive/20"
          >
            <ConfirmActionButton
              variant="destructive"
              size="sm"
              disabled={remove.isPending}
              onConfirm={() => remove.mutateAsync(id, { onSuccess: onBack })}
            >
              <Trash2 className="mr-1.5 h-4 w-4" />
              {t("productDetail.delete")}
            </ConfirmActionButton>
          </DetailSection>
        </TabsContent>
        <TabsContent value="related" className="mt-0">
          <RelatedGeneratedAssets
            organizationId={product.organization_id || organization.id}
            sourceProductId={id}
            scope={product.scope}
          />
        </TabsContent>
      </Tabs>
    </DetailPage>
  );
}
