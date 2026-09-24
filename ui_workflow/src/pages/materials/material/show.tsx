import {
  ArrowLeft,
  Braces,
  Edit3,
  FileImage,
  Info,
  Sparkles,
  Trash2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
  StructuredDetails,
} from "@/foundation/components/detail-layout";
import { LibraryGallery } from "@/foundation/components/library-gallery";
import { LibraryVideo } from "@/foundation/components/library-image";
import { hostShim } from "@/foundation/lib/host-shim";

function recordOf(
  value: Record<string, unknown> | string | undefined,
): Record<string, unknown> {
  if (!value) return {};
  if (typeof value !== "string") return value;
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return { value };
  }
}

export function MaterialsShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const { data: material, isLoading } = useMaterial(id);
  const remove = useDeleteMaterial();
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!material)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {t("materialDetail.notFound")}
      </div>
    );
  const metadata = recordOf(material.metadata);
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center justify-between rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("materialDetail.back")}
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          {t("materialDetail.edit")}
        </Button>
      </div>
      <section className="grid gap-6 lg:grid-cols-[minmax(300px,460px)_1fr] lg:items-start">
        {material.asset_kind === "movement" && material.object_path ? (
          <div className="aspect-video overflow-hidden rounded-xl border bg-black">
            <LibraryVideo
              organizationId={material.organization_id ?? organization.id}
              objectPath={material.object_path}
            />
          </div>
        ) : (
          <LibraryGallery
            organizationId={material.organization_id ?? organization.id}
            items={[
              {
                objectPath: material.object_path,
                label: material.asset_kind || t("materialForm.image"),
              },
            ]}
            alt={material.name}
          />
        )}
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
              {t(`materialsList.kind.${material.asset_kind}`, {
                defaultValue: material.asset_kind,
              })}
            </span>
            <VisibilityBadge visibility={material.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              {t("materialDetail.type")}
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {material.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {material.description || t("materialDetail.noDescription")}
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label={t("materialDetail.scope")}>
              {material.scope}
            </DetailField>
            <DetailField label={t("materialDetail.owner")}>
              {material.owner_user_id}
            </DetailField>
            <DetailField label={t("materialDetail.createdAt")}>
              {formatDetailDate(material.created_at)}
            </DetailField>
            <DetailField label={t("materialDetail.updatedAt")}>
              {formatDetailDate(material.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <div className="grid gap-5 lg:grid-cols-2">
        {material.prompt ? (
          <DetailSection
            title={t("materialDetail.prompt")}
            description={t("materialDetail.promptHint")}
            icon={Sparkles}
          >
            <p className="whitespace-pre-wrap text-sm leading-7">
              {material.prompt}
            </p>
          </DetailSection>
        ) : null}
        {material.content ? (
          <DetailSection
            title={t("materialForm.textContent")}
            description={t("materialDetail.textHint")}
            icon={Info}
          >
            <p className="whitespace-pre-wrap text-sm leading-7">
              {material.content}
            </p>
          </DetailSection>
        ) : null}
      </div>
      {Object.keys(metadata).length ? (
        <DetailSection
          title={t("materialDetail.metadata")}
          description={t("materialDetail.metadataHint")}
          icon={Braces}
        >
          <StructuredDetails value={metadata} />
        </DetailSection>
      ) : null}
      <DetailSection
        title={t("materialDetail.management")}
        description={t("materialDetail.managementHint")}
        icon={FileImage}
        className="border-destructive/20"
      >
        <Button
          variant="destructive"
          size="sm"
          disabled={remove.isPending}
          onClick={() => remove.mutate(id, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          {t("materialDetail.delete")}
        </Button>
      </DetailSection>
    </DetailPage>
  );
}
