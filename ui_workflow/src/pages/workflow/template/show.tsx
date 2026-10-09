import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Pencil, Send, Settings2 } from "lucide-react";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import {
  useArchiveTemplate,
  useChangeTemplateVisibility,
  usePublishTemplate,
} from "@/domains/templates/hooks/use-template-mutations";
import {
  getTemplateVersion,
  grantTemplate,
  revokeTemplate,
} from "@/domains/templates/lib/api";
import { InvocationMappingView } from "@/domains/templates/components/invocation-mapping-view";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { ListState } from "@/foundation/components/resource-list-layout";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { hostShim } from "@/foundation/lib/host-shim";
import { ResourceSharing } from "@/foundation/components/resource-sharing";

export function TemplateShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const query = useTemplate(id);
  const { user } = hostShim.useCurrentUser();
  const shell = hostShim.useShellContext();
  const archive = useArchiveTemplate();
  const visibility = useChangeTemplateVisibility();
  const publish = usePublishTemplate();
  const [version, setVersion] = useState<number | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const historical = useQuery({
    queryKey: ["template-version", id, version],
    queryFn: () => getTemplateVersion(id, version!),
    enabled: version !== null,
  });
  const template = query.data;
  if (query.isLoading) return <ListState loading />;
  if (query.isError)
    return (
      <ListState
        error={query.error.message}
        onRetry={() => void query.refetch()}
      />
    );
  if (!template) return <ListState empty={t("templateView.notFound")} />;
  const owner = Boolean(user?.id && user.id === template.created_by);
  const displayed = version === null ? template : historical.data;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <AppStudioHeader>
        <div className="flex min-w-0 items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("templateView.back")}
            onClick={onBack}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h1 className="truncate text-base font-semibold">{template.name}</h1>
          <span className="text-xs text-muted-foreground">
            v{version ?? template.version}
          </span>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSettingsOpen(true)}
          >
            <Settings2 className="mr-1 h-4 w-4" />
            {t("mapping.settings")}
          </Button>
          {owner && version === null && template.status === "draft" ? (
            <Button
              size="sm"
              disabled={publish.isPending}
              onClick={() => publish.mutate(id)}
            >
              <Send className="mr-1 h-4 w-4" />
              {t("templateView.publish")}
            </Button>
          ) : null}
          {version === null && template.status !== "archived" ? (
            <Button variant="outline" size="sm" onClick={onEdit}>
              <Pencil className="mr-1 h-4 w-4" />
              {t("templateView.edit")}
            </Button>
          ) : null}
        </div>
      </AppStudioHeader>
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="mb-5 flex flex-wrap items-center gap-3">
          <span className="rounded-full bg-primary/10 px-3 py-1 text-xs">
            {t(`v12.definitionStatus.${displayed?.status ?? "unknown"}`)}
          </span>
          <span className="text-xs text-muted-foreground">
            {template.scope}
          </span>
          <label htmlFor="template-view-version" className="text-xs">
            {t("templateView.version")}
          </label>
          <select
            id="template-view-version"
            className="h-8 rounded-lg border bg-background px-2 text-xs"
            value={version ?? template.version}
            onChange={(event) => {
              const selected = Number(event.target.value);
              setVersion(selected === template.version ? null : selected);
            }}
          >
            {Array.from(
              { length: template.version },
              (_, index) => template.version - index,
            ).map((number) => (
              <option key={number} value={number}>
                v{number}
                {number === template.version
                  ? ` · ${t("templateView.current")}`
                  : ""}
              </option>
            ))}
          </select>
          {version !== null ? (
            <span className="text-xs text-muted-foreground">
              {t("templateView.historical")}
            </span>
          ) : null}
        </div>
        {publish.error ? (
          <p role="alert" className="mb-4 text-sm text-destructive">
            {publish.error.message}
          </p>
        ) : null}
        {version !== null && historical.isLoading ? (
          <ListState loading />
        ) : version !== null && historical.isError ? (
          <ListState
            error={historical.error.message}
            onRetry={() => void historical.refetch()}
          />
        ) : displayed ? (
          <div className="template-detail-layout">
            <section className="space-y-4">
              <h2 className="font-semibold">{t("templateFields.prompt")}</h2>
              <p className="text-xs leading-5 text-muted-foreground">
                {displayed.description || t("v12.noDescription")}
              </p>
              <div className="whitespace-pre-wrap rounded-2xl bg-background p-6 text-sm leading-7">
                {displayed.prompt_body}
              </div>
              <h2 className="pt-3 font-semibold">{t("v12.inputs")}</h2>
              <div className="grid gap-3 sm:grid-cols-2">
                {displayed.inputs.map((slot) => (
                  <article
                    key={slot.name}
                    className="rounded-xl border bg-background p-4"
                  >
                    <h3 className="text-sm font-semibold">
                      {slot.name}
                      {slot.required ? " *" : ""}
                    </h3>
                    <p className="mt-2 text-xs text-muted-foreground">
                      {slot.asset_kind} · {slot.resource_type}
                    </p>
                  </article>
                ))}
              </div>
            </section>
            <aside className="space-y-6">
              <section className="rounded-2xl bg-background p-5">
                <h2 className="font-semibold">
                  {t("templateEditor.capability")}
                </h2>
                <p className="mt-3 break-all text-sm">
                  {displayed.capability_key}@{displayed.capability_version}
                </p>
                <p className="mt-3 text-xs text-muted-foreground">
                  {t("templateFields.outputKind")} ·{" "}
                  {displayed.output_asset_kind}
                </p>
              </section>
              <InvocationMappingView
                value={displayed.invocation_mapping ?? {}}
              />
            </aside>
          </div>
        ) : null}
      </div>
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("mapping.settings")}</DialogTitle>
            <DialogDescription>{t("mapping.settingsHint")}</DialogDescription>
          </DialogHeader>
          <p className="text-sm">
            {template.name} · {template.scope}
          </p>
          <ResourceSharing
            organizationId={shell.organization.id}
            visibility={template.visibility}
            owner={owner}
            onVisibility={(value) =>
              visibility.mutateAsync({ id, input: { visibility: value } })
            }
            onGrant={(team, permission) =>
              grantTemplate(id, { team_id: team, permission })
            }
            onRevoke={(team) => revokeTemplate(id, team)}
          />
          {template.status !== "archived" ? (
            <Button
              variant="destructive"
              onClick={() => {
                setSettingsOpen(false);
                setArchiveOpen(true);
              }}
            >
              {t("templateView.archive")}
            </Button>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog
        open={archiveOpen}
        onOpenChange={(open) => {
          if (!archive.isPending) setArchiveOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("templateView.archive")}</DialogTitle>
            <DialogDescription>
              {t("templateView.archiveHint")}
            </DialogDescription>
          </DialogHeader>
          {archive.error ? (
            <p role="alert" className="text-sm text-destructive">
              {archive.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={archive.isPending}
              onClick={() => setArchiveOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={archive.isPending}
              onClick={() =>
                archive.mutate(id, { onSuccess: () => setArchiveOpen(false) })
              }
            >
              {t("templateView.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
