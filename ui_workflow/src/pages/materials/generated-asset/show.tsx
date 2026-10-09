import { useTranslation } from "react-i18next";
import { ListState } from "@/foundation/components/resource-list-layout";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import { ArrowLeft, Boxes, GitBranch, ImageIcon, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset";
import { useDeleteGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset-mutations";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { LibraryMedia } from "@/foundation/components/library-media";
import { hostShim } from "@/foundation/lib/host-shim";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";

export function GeneratedAssetShow({
  id,
  onBack,
}: {
  id: string;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const {
    data: asset,
    isLoading,
    isError,
    error,
    refetch,
  } = useGeneratedAsset(id);
  const remove = useDeleteGeneratedAsset();
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (!asset)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {t("library.empty")}
      </div>
    );
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("library.assetBack")}
        </Button>
      </div>
      <section className="media-detail-grid items-start">
        <div className="relative aspect-[4/3] overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br from-muted/70 via-muted/30 to-background shadow-sm">
          {asset.object_path || asset.url ? (
            <LibraryMedia
              organizationId={organization.id}
              objectPath={asset.object_path || asset.url}
              alt={asset.name}
              video={asset.asset_kind === "video"}
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
              <ImageIcon className="h-10 w-10" />
              <span className="text-sm">{t("library.noMedia")}</span>
            </div>
          )}
          <span className="absolute bottom-3 left-3 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white backdrop-blur">
            {asset.asset_kind}
          </span>
        </div>
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
              {t("nav.generated")}
            </span>
            <VisibilityBadge visibility={asset.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              {t("library.assetTitle")}
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {asset.name}
            </h1>
            <p className="mt-3 text-sm leading-7 text-muted-foreground">
              {t("library.assetHint")}
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label={t("library.fields.scope")}>
              {asset.scope}
            </DetailField>
            <DetailField label={t("library.fields.owner")}>
              {asset.owner_user_id}
            </DetailField>
            <DetailField label={t("library.fields.kind")}>
              {asset.asset_kind}
            </DetailField>
            <DetailField label={t("library.fields.created")}>
              {formatDetailDate(asset.created_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      {asset.content ? (
        <DetailSection
          title={t("library.content")}
          description={t("library.assetHint")}
          icon={Boxes}
        >
          <p className="whitespace-pre-wrap text-sm leading-7">
            {asset.content}
          </p>
        </DetailSection>
      ) : null}
      <DetailSection
        title={t("library.source")}
        description={t("library.assetHint")}
        icon={GitBranch}
      >
        <DetailFieldGrid>
          <DetailField label={t("library.fields.job")}>
            {asset.job_id}
          </DetailField>
          <DetailField label={t("library.fields.step")}>
            {asset.step_run_id}
          </DetailField>
          <DetailField label={t("library.fields.template")}>
            {asset.template_id}
          </DetailField>
          <DetailField label={t("library.fields.version")}>
            v{asset.template_version}
          </DetailField>
          <DetailField label={t("library.fields.virtualIP")}>
            {asset.source_virtual_ip_id}
          </DetailField>
          <DetailField label={t("library.fields.product")}>
            {asset.source_product_id}
          </DetailField>
        </DetailFieldGrid>
      </DetailSection>
      <DetailSection
        title={t("library.management")}
        description={t("library.assetHint")}
        icon={Boxes}
        className="border-destructive/20"
      >
        <ConfirmActionButton
          variant="destructive"
          size="sm"
          disabled={remove.isPending}
          onConfirm={() => remove.mutateAsync(id, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          {t("library.deleteAsset")}
        </ConfirmActionButton>
      </DetailSection>
    </DetailPage>
  );
}
