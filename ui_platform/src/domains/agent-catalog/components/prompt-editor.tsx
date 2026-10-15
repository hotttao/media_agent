import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import {
  catalogPath,
  saveResource,
  versions,
} from "@/domains/agent-catalog/lib/catalog-client";
import type {
  CatalogResource,
  ContentVersion,
} from "@/domains/agent-catalog/types";
import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";
import { CatalogError, ConfirmAction, Field, inputClass } from "./shared";
export function PromptEditor({
  resource,
  saved,
}: {
  resource: CatalogResource;
  saved(value: CatalogResource): void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(resource.name);
  const [content, setContent] = useState(resource.content ?? "");
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const loadVersions = useCallback(
    () => versions("prompts", resource.id),
    [resource.id],
  );
  const history = useCatalogLoad(loadVersions);
  const loadVersion = useCallback(
    () =>
      version
        ? managementRequest<ContentVersion>(
            `${catalogPath("prompts", resource.id)}/versions/${version}`,
          )
        : Promise.resolve({
            content: resource.content,
            name: resource.name,
            version: resource.version,
          }),
    [resource.id, resource.content, resource.name, resource.version, version],
  );
  const selected = useCatalogLoad(loadVersion);
  useEffect(() => {
    if (selected.data) {
      setName(selected.data.name ?? resource.name);
      setContent(selected.data.content ?? "");
    }
  }, [selected.data, resource.name]);
  const editable =
    resource.my_permission !== "viewer" && resource.status !== "archived";
  const dirty = name !== resource.name || content !== (resource.content ?? "");
  function select(value: number) {
    if (!version && dirty && !window.confirm(t("catalog.discardEdits"))) return;
    setVersion(value);
  }
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="rounded bg-amber-100 px-2 py-1 text-xs text-amber-900">
          {t("catalog.fixedUpdates")}
        </span>
        <select
          aria-label={t("catalog.history")}
          value={version}
          className={`${inputClass} max-w-52`}
          onChange={(event) => select(Number(event.target.value))}
        >
          <option value={0}>
            {t("catalog.currentVersion", { version: resource.version })}
          </option>
          {history.data?.map((item) => (
            <option key={item.version} value={item.version}>
              {t("catalog.version", { version: item.version })}
            </option>
          ))}
        </select>
      </div>
      <CatalogError error={history.error} retry={history.reload} />
      <CatalogError error={selected.error} retry={selected.reload} />
      <form
        className="grid gap-4"
        onSubmit={async (event) => {
          event.preventDefault();
          if (!editable || version || busy) return;
          setBusy(true);
          setError(undefined);
          try {
            const value = await saveResource("prompts", resource, {
              name,
              content,
            });
            saved(value);
            history.reload();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Field label={t("catalog.name")}>
          <input
            className={inputClass}
            value={name}
            onChange={(event) => setName(event.target.value)}
            disabled={!editable || !!version || busy}
            required
            maxLength={200}
          />
        </Field>
        <Field label={t("catalog.promptBody")}>
          <textarea
            className={`${inputClass} min-h-[360px] resize-y font-mono leading-6`}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            readOnly={!editable || !!version}
            disabled={selected.loading || busy}
          />
        </Field>
        <CatalogError error={error} />
        <div className="flex flex-wrap justify-end gap-2">
          {editable && !version && (
            <Button disabled={busy || !dirty || !name.trim()}>
              {t(busy ? "catalog.saving" : "catalog.save")}
            </Button>
          )}
          {editable && version > 0 && (
            <ConfirmAction
              title={t("catalog.rollback")}
              description={t("catalog.rollbackConfirm", { version })}
              onError={setError}
              action={async () => {
                const value = await managementWrite<CatalogResource>(
                  `${catalogPath("prompts", resource.id)}/rollback`,
                  "POST",
                  {
                    target_version: version,
                    expected_version: resource.version,
                  },
                );
                saved(value);
                setVersion(0);
                history.reload();
              }}
            />
          )}
        </div>
      </form>
    </div>
  );
}
