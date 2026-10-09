import { useTranslation } from "react-i18next";
import { ResourceSharing } from "@/foundation/components/resource-sharing";
import { ListState } from "@/foundation/components/resource-list-layout";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import {
  Archive,
  ArrowLeft,
  BriefcaseBusiness,
  Edit3,
  Heart,
  Ruler,
  Trash2,
  UserRound,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip";
import {
  useArchiveVirtualIp,
  useChangeVirtualIpVisibility,
  useDeleteVirtualIp,
  useGrantVirtualIp,
  useRevokeVirtualIp,
} from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type { PersonaProfile } from "@/domains/virtual-ips/types";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";
import { LibraryGallery } from "@/foundation/components/library-gallery";
import { hostShim } from "@/foundation/lib/host-shim";

function profileOf(value: PersonaProfile | string | undefined): PersonaProfile {
  if (!value) return {};
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as PersonaProfile;
  } catch {
    return {};
  }
}
export function VirtualIpShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const { user } = hostShim.useCurrentUser();
  const {
    data: virtualIp,
    isLoading,
    isError,
    error,
    refetch,
  } = useVirtualIp(id);
  const remove = useDeleteVirtualIp();
  const archive = useArchiveVirtualIp();
  const changeVisibility = useChangeVirtualIpVisibility();
  const grant = useGrantVirtualIp();
  const revoke = useRevokeVirtualIp();
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (!virtualIp)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {t("library.virtualEmpty")}
      </div>
    );
  const profile = profileOf(virtualIp.persona_profile);
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("library.virtualBack")}
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          {t("library.editVirtual")}
        </Button>
      </div>
      <section className="media-detail-grid items-start">
        <LibraryGallery
          organizationId={virtualIp.organization_id ?? organization.id}
          items={[
            {
              objectPath: virtualIp.avatar_object_path,
              label: t("virtualProfile.avatar_object_path"),
              badge: virtualIp.avatar_object_path
                ? t("virtualProfile.avatar_object_path")
                : t("library.noMedia"),
            },
            {
              objectPath: virtualIp.full_body_object_path,
              label: t("virtualProfile.full_body_object_path"),
              badge: virtualIp.full_body_object_path
                ? t("virtualProfile.full_body_object_path")
                : t("library.noMedia"),
            },
            {
              objectPath: virtualIp.three_view_object_path,
              label: t("virtualProfile.three_view_object_path"),
              badge: virtualIp.three_view_object_path
                ? t("virtualProfile.three_view_object_path")
                : t("library.noMedia"),
            },
            {
              objectPath: virtualIp.nine_view_object_path,
              label: t("virtualProfile.nine_view_object_path"),
              badge: virtualIp.nine_view_object_path
                ? t("virtualProfile.nine_view_object_path")
                : t("library.noMedia"),
            },
          ]}
          alt={virtualIp.name}
          showEmptyItems
        />
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium capitalize text-primary">
              {t(`common.status.${virtualIp.status}`)}
            </span>
            <VisibilityBadge visibility={virtualIp.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              {t("library.virtualTitle")}
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {virtualIp.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {virtualIp.description || profile.basic_setting || "—"}
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label={t("library.fields.scope")}>
              {virtualIp.scope}
            </DetailField>
            <DetailField label={t("library.fields.owner")}>
              {virtualIp.owner_user_id}
            </DetailField>
            <DetailField label={t("library.fields.created")}>
              {formatDetailDate(virtualIp.created_at)}
            </DetailField>
            <DetailField label={t("registry.updated")}>
              {formatDetailDate(virtualIp.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <div className="grid gap-5 xl:grid-cols-2">
        <DetailSection
          title={t("virtualDetail.basic")}
          description={t("virtualProfile.profileHint")}
          icon={UserRound}
        >
          <DetailFieldGrid>
            <DetailField label={t("virtualProfile.gender")}>
              {profile.gender
                ? t(`virtualProfile.${profile.gender}`)
                : undefined}
            </DetailField>
            <DetailField label={t("virtualProfile.age")}>
              {profile.age === undefined
                ? undefined
                : `${profile.age} ${t("virtualProfile.years")}`}
            </DetailField>
            <DetailField label={t("virtualProfile.city")}>
              {profile.city}
            </DetailField>
            <DetailField label={t("virtualProfile.occupation")}>
              {profile.occupation}
            </DetailField>
            <DetailField label={t("virtualProfile.education")}>
              {profile.education}
            </DetailField>
            <DetailField label={t("virtualProfile.major")}>
              {profile.major}
            </DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title={t("virtualDetail.body")}
          description={t("virtualDetail.bodyHint")}
          icon={Ruler}
        >
          <DetailFieldGrid>
            <DetailField label={t("virtualProfile.height")}>
              {profile.height === undefined
                ? undefined
                : `${profile.height} cm`}
            </DetailField>
            <DetailField label={t("virtualProfile.weight")}>
              {profile.weight === undefined
                ? undefined
                : `${profile.weight} kg`}
            </DetailField>
            <DetailField label={t("virtualProfile.bust")}>
              {profile.bust === undefined ? undefined : `${profile.bust} cm`}
            </DetailField>
            <DetailField label={t("virtualProfile.waist")}>
              {profile.waist === undefined ? undefined : `${profile.waist} cm`}
            </DetailField>
            <DetailField label={t("virtualProfile.hip")}>
              {profile.hip === undefined ? undefined : `${profile.hip} cm`}
            </DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title={t("virtualDetail.background")}
          description={t("virtualProfile.profileHint")}
          icon={BriefcaseBusiness}
        >
          <DetailFieldGrid className="sm:grid-cols-1">
            <DetailField label={t("virtualProfile.basic_setting")}>
              {profile.basic_setting}
            </DetailField>
            <DetailField label={t("virtualProfile.family_background")}>
              {profile.family_background}
            </DetailField>
            <DetailField label={t("virtualProfile.income_level")}>
              {profile.income_level}
            </DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title={t("virtualDetail.expression")}
          description={t("virtualProfile.profileHint")}
          icon={Heart}
        >
          <DetailFieldGrid className="sm:grid-cols-1">
            <DetailField label={t("virtualProfile.personality")}>
              {profile.personality}
            </DetailField>
            <DetailField label={t("virtualProfile.catchphrase")}>
              {profile.catchphrase}
            </DetailField>
            <DetailField label={t("virtualProfile.small_habit")}>
              {profile.small_habit}
            </DetailField>
            <DetailField label={t("virtualProfile.hobbies")}>
              {profile.hobbies}
            </DetailField>
          </DetailFieldGrid>
        </DetailSection>
      </div>
      <ResourceSharing
        organizationId={virtualIp.organization_id || organization.id}
        visibility={virtualIp.visibility}
        owner={Boolean(user?.id && user.id === virtualIp.owner_user_id)}
        permissions={["viewer"]}
        onVisibility={(visibility) =>
          changeVisibility.mutateAsync({ id, input: { visibility } })
        }
        onGrant={(team) => grant.mutateAsync({ id, grant: { team_id: team } })}
        onRevoke={(teamId) => revoke.mutateAsync({ id, teamId })}
      />
      <DetailSection
        title={t("library.management")}
        description={t("virtualDetail.managementHint")}
        icon={Archive}
        className="border-destructive/20"
      >
        <div className="flex flex-wrap gap-2">
          {virtualIp.status !== "archived" ? (
            <ConfirmActionButton
              variant="outline"
              disabled={archive.isPending}
              onConfirm={() => archive.mutateAsync(id)}
            >
              <Archive className="mr-1.5 h-4 w-4" />
              {t("productDetail.archive")}
            </ConfirmActionButton>
          ) : null}
          <ConfirmActionButton
            variant="destructive"
            disabled={remove.isPending}
            onConfirm={() => remove.mutateAsync(id, { onSuccess: onBack })}
          >
            <Trash2 className="mr-1.5 h-4 w-4" />
            {t("common.delete")}
          </ConfirmActionButton>
        </div>
      </DetailSection>
    </DetailPage>
  );
}
