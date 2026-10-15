import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import {
  catalogPath,
  listDefinitions,
} from "@/domains/agent-catalog/lib/catalog-client";
import type { MCPDefinition } from "@/domains/agent-catalog/types";
import { managementWrite } from "@/foundation/lib/management-http";
import { CatalogError, CatalogModal, Field, inputClass } from "./shared";

function DefinitionForm({
  organization,
  platform,
  value,
  close,
  saved,
}: {
  organization: string;
  platform: boolean;
  value?: MCPDefinition;
  close(): void;
  saved(): void;
}) {
  const { t } = useTranslation();
  const [id, setID] = useState(value?.definition_id ?? "");
  const [service, setService] = useState(value?.service_name ?? "");
  const [endpoint, setEndpoint] = useState(value?.endpoint ?? "");
  const [transport, setTransport] = useState(
    value?.transport ?? "streamable_http",
  );
  const [auth, setAuth] = useState(value?.authentication_type ?? "none");
  const [status, setStatus] = useState(value?.status ?? "enabled");
  const [platformScope, setPlatformScope] = useState(
    value ? !value.organization_id : false,
  );
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  return (
    <CatalogModal
      title={t(value ? "catalog.editMCP" : "catalog.newMCP")}
      close={() => {
        if (!busy) close();
      }}
    >
      <form
        className="grid gap-4"
        onSubmit={async (event) => {
          event.preventDefault();
          setBusy(true);
          try {
            await managementWrite(
              catalogPath("mcp", value?.definition_id),
              value ? "PUT" : "POST",
              {
                definition_id: id,
                organization_id: platformScope ? "" : organization,
                service_name: service,
                endpoint,
                transport,
                authentication_type: auth,
                status,
                expected_version: value?.version ?? 0,
              },
            );
            saved();
            close();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        {platform && !value && (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={platformScope}
              onChange={(event) => setPlatformScope(event.target.checked)}
            />
            {t("catalog.platformScope")}
          </label>
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("catalog.definitionID")}>
            <input
              className={inputClass}
              value={id}
              disabled={!!value}
              onChange={(event) => setID(event.target.value)}
              required
              pattern="[a-z][a-z0-9_-]{0,47}"
            />
          </Field>
          <Field label={t("catalog.serviceName")}>
            <input
              className={inputClass}
              value={service}
              onChange={(event) => setService(event.target.value)}
              required
              pattern="[a-z][a-z0-9_-]{0,47}"
            />
          </Field>
        </div>
        <Field label={t("catalog.endpoint")}>
          <input
            className={inputClass}
            type={transport === "platform" ? "text" : "url"}
            value={endpoint}
            onChange={(event) => setEndpoint(event.target.value)}
            required
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("catalog.transport")}>
            <select
              className={inputClass}
              value={transport}
              disabled={value?.transport === "platform"}
              onChange={(event) =>
                setTransport(event.target.value as MCPDefinition["transport"])
              }
            >
              <option value="streamable_http">Streamable HTTP</option>
              <option value="sse">SSE</option>
              {value?.transport === "platform" && (
                <option value="platform">{t("catalog.builtin")}</option>
              )}
            </select>
          </Field>
          <Field label={t("catalog.authentication")}>
            <select
              className={inputClass}
              value={auth}
              disabled={value?.transport === "platform"}
              onChange={(event) =>
                setAuth(
                  event.target.value as MCPDefinition["authentication_type"],
                )
              }
            >
              {[
                "none",
                "bearer",
                "api_key",
                "oauth",
                ...(platformScope ? ["session_context"] : []),
              ].map((type) => (
                <option key={type} value={type}>
                  {t(`catalog.auth_${type}`)}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <Field label={t("catalog.status")}>
          <select
            className={inputClass}
            value={status}
            disabled={value?.status === "retired"}
            onChange={(event) =>
              setStatus(event.target.value as MCPDefinition["status"])
            }
          >
            {["enabled", "disabled", "retired"].map((item) => (
              <option key={item} value={item}>
                {t(`catalog.${item}`)}
              </option>
            ))}
          </select>
        </Field>
        <p className="text-xs leading-5 text-stone-500">
          {t("catalog.mcpRegistrationHint")}
        </p>
        <CatalogError error={error} />
        <div className="flex justify-end">
          <Button disabled={busy}>
            {t(busy ? "catalog.saving" : "catalog.save")}
          </Button>
        </div>
      </form>
    </CatalogModal>
  );
}
export function MCPCatalog({
  organization,
  admin,
  platformAdmin,
}: {
  organization: string;
  admin: boolean;
  platformAdmin: boolean;
}) {
  const { t } = useTranslation();
  const load = useCallback(() => listDefinitions(organization), [organization]);
  const request = useCatalogLoad(load);
  const [edit, setEdit] = useState<MCPDefinition | null | undefined>();
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-5 sm:p-8">
      <header className="mb-6 flex items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">MCP</h1>
          <p className="mt-2 text-sm text-stone-500">{t("catalog.mcpHint")}</p>
        </div>
        {(admin || platformAdmin) && (
          <Button onClick={() => setEdit(null)}>{t("catalog.newMCP")}</Button>
        )}
      </header>
      <CatalogError error={request.error} retry={request.reload} />
      {request.loading && (
        <p className="text-sm text-stone-500">{t("catalog.loading")}</p>
      )}
      <div className="grid gap-3 lg:grid-cols-2">
        {request.data?.map((item) => (
          <article
            key={item.definition_id}
            className="rounded-xl border border-stone-200 bg-white p-5"
          >
            <div className="flex items-start justify-between gap-2">
              <div>
                <h2 className="font-semibold">{item.service_name}</h2>
                <p className="mt-1 text-xs text-stone-500">
                  {item.definition_id} ·{" "}
                  {t("catalog.version", { version: item.version })}
                </p>
              </div>
              <span
                className={`rounded px-2 py-1 text-xs ${item.status === "enabled" ? "bg-emerald-50 text-emerald-700" : "bg-stone-100 text-stone-600"}`}
              >
                {t(`catalog.${item.status}`)}
              </span>
            </div>
            <p className="my-4 break-all font-mono text-xs text-stone-500">
              {item.transport === "platform"
                ? t("catalog.builtin")
                : item.endpoint}
            </p>
            <div className="flex items-center justify-between gap-2 text-xs">
              <span>
                {t(`catalog.auth_${item.authentication_type}`)} ·{" "}
                {t(
                  item.organization_id
                    ? "catalog.enterpriseScope"
                    : "catalog.platformScope",
                )}
              </span>
              {((item.organization_id === organization && admin) ||
                (!item.organization_id && platformAdmin)) && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setEdit(item)}
                >
                  {t("catalog.edit")}
                </Button>
              )}
            </div>
          </article>
        ))}
      </div>
      {!request.loading && !request.error && !request.data?.length && (
        <p className="py-16 text-center text-stone-500">{t("catalog.empty")}</p>
      )}
      {edit !== undefined && (
        <DefinitionForm
          organization={organization}
          platform={platformAdmin}
          value={edit ?? undefined}
          close={() => setEdit(undefined)}
          saved={request.reload}
        />
      )}
    </div>
  );
}
