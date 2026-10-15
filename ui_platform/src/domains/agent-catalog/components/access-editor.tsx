import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { catalogPath } from "@/domains/agent-catalog/lib/catalog-client";
import type { CatalogResource, Grant } from "@/domains/agent-catalog/types";
import { managementWrite } from "@/foundation/lib/management-http";
import type { Team } from "@/foundation/types/hierarchy";
import { CatalogError, CatalogModal, Field, inputClass } from "./shared";
export function AccessEditor({
  resource,
  kind,
  teams,
  close,
  saved,
}: {
  resource: CatalogResource;
  kind: "prompts" | "skills";
  teams: Team[];
  close(): void;
  saved(value: CatalogResource): void;
}) {
  const { t } = useTranslation();
  const [visibility, setVisibility] = useState(resource.visibility);
  const [grants, setGrants] = useState<Grant[]>(resource.grants ?? []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  return (
    <CatalogModal
      title={t("catalog.accessTitle", { name: resource.name })}
      close={() => {
        if (!busy) close();
      }}
    >
      <form
        className="grid gap-4"
        onSubmit={async (event) => {
          event.preventDefault();
          setBusy(true);
          setError(undefined);
          try {
            const value = await managementWrite<CatalogResource>(
              `${catalogPath(kind, resource.id)}/access`,
              "PUT",
              {
                visibility,
                grants: visibility === "private" ? [] : grants,
                expected_version: resource.version,
              },
            );
            saved(value);
            close();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Field label={t("catalog.visibility")}>
          <select
            className={inputClass}
            value={visibility}
            onChange={(event) =>
              setVisibility(event.target.value as typeof visibility)
            }
          >
            {(["private", "team", "public"] as const).map((value) => (
              <option key={value} value={value}>
                {t(`catalog.visibility_${value}`)}
              </option>
            ))}
          </select>
        </Field>
        {visibility !== "private" && (
          <>
            <div className="grid gap-2">
              {grants.map((grant, index) => (
                <div key={grant.team_id} className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {teams.find((team) => team.id === grant.team_id)?.name ??
                      grant.team_id}
                  </span>
                  <select
                    aria-label={t("catalog.permission")}
                    className={inputClass}
                    value={grant.permission}
                    onChange={(event) =>
                      setGrants((values) =>
                        values.map((item, position) =>
                          position === index
                            ? {
                                ...item,
                                permission: event.target
                                  .value as Grant["permission"],
                              }
                            : item,
                        ),
                      )
                    }
                  >
                    <option value="viewer">{t("catalog.viewer")}</option>
                    <option value="editor">{t("catalog.editor")}</option>
                  </select>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() =>
                      setGrants((values) =>
                        values.filter((_, position) => position !== index),
                      )
                    }
                  >
                    {t("catalog.remove")}
                  </Button>
                </div>
              ))}
            </div>
            <Field label={t("catalog.addTeam")}>
              <select
                className={inputClass}
                value=""
                onChange={(event) => {
                  if (event.target.value)
                    setGrants((values) => [
                      ...values,
                      { team_id: event.target.value, permission: "viewer" },
                    ]);
                }}
              >
                <option value="">{t("catalog.chooseTeam")}</option>
                {teams
                  .filter(
                    (team) =>
                      team.organization_id === resource.organization_id &&
                      !grants.some((grant) => grant.team_id === team.id),
                  )
                  .map((team) => (
                    <option key={team.id} value={team.id}>
                      {team.name}
                    </option>
                  ))}
              </select>
            </Field>
          </>
        )}
        <p className="text-xs leading-5 text-stone-500">
          {t("catalog.accessHint")}
        </p>
        <CatalogError error={error} />
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={busy}
          >
            {t("catalog.cancel")}
          </Button>
          <Button
            disabled={busy || (visibility === "team" && grants.length === 0)}
          >
            {t(busy ? "catalog.saving" : "catalog.save")}
          </Button>
        </div>
      </form>
    </CatalogModal>
  );
}
