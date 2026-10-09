import { useTranslation } from "react-i18next";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import { ListState } from "@/foundation/components/resource-list-layout";
import { ArrowLeft, Database, Edit3, Layers3, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind";
import { useDeleteAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";

export function AssetKindShow({
  name,
  onBack,
  onEdit,
}: {
  name: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const {
    data: assetKind,
    isLoading,
    isError,
    error,
    refetch,
  } = useAssetKind(name);
  const remove = useDeleteAssetKind();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (!assetKind)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {t("library.empty")}
      </div>
    );
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center justify-between rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("registry.back")}
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          {t("registry.edit")}
        </Button>
      </div>
      <section className="rounded-2xl border border-border/70 bg-gradient-to-br from-primary/10 via-card to-card p-6 shadow-sm sm:p-8">
        <div className="flex items-start gap-4">
          <div className="rounded-2xl bg-primary/10 p-3 text-primary">
            <Layers3 className="h-7 w-7" />
          </div>
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap gap-2">
              <span className="rounded-full bg-background/80 px-2.5 py-1 text-xs font-medium">
                {assetKind.resource_type}
              </span>
              <span className="rounded-full bg-background/80 px-2.5 py-1 text-xs font-medium">
                {assetKind.media_type}
              </span>
              <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
                {assetKind.status}
              </span>
            </div>
            <h1 className="break-words text-3xl font-semibold tracking-tight sm:text-4xl">
              {assetKind.name}
            </h1>
            <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
              {assetKind.description || "—"}
            </p>
          </div>
        </div>
      </section>
      <DetailSection
        title={t("registry.definition")}
        description={t("registry.hint")}
        icon={Database}
      >
        <DetailFieldGrid>
          <DetailField label={t("registry.resourceType")}>
            {assetKind.resource_type}
          </DetailField>
          <DetailField label={t("registry.mediaType")}>
            {assetKind.media_type}
          </DetailField>
          <DetailField label={t("registry.source")}>
            {assetKind.source_table || "—"}
          </DetailField>
          <DetailField label={t("registry.status")}>
            {assetKind.status}
          </DetailField>
          <DetailField label={t("library.fields.created")}>
            {formatDetailDate(assetKind.created_at)}
          </DetailField>
          <DetailField label={t("registry.updated")}>
            {formatDetailDate(assetKind.updated_at)}
          </DetailField>
        </DetailFieldGrid>
      </DetailSection>
      <DetailSection
        title={t("library.management")}
        description={
          assetKind.status === "SYSTEM"
            ? t("registry.systemHint")
            : t("registry.deleteHint")
        }
        icon={Layers3}
        className="border-destructive/20"
      >
        <ConfirmActionButton
          variant="destructive"
          size="sm"
          disabled={remove.isPending || assetKind.status === "SYSTEM"}
          onConfirm={() => remove.mutateAsync(name, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          {t("common.delete")}
        </ConfirmActionButton>
      </DetailSection>
    </DetailPage>
  );
}
