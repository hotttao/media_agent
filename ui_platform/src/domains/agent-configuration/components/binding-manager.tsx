import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  bindingsPath,
  listBindings,
} from "@/domains/agent-configuration/lib/configuration-client";
import type {
  Binding,
  DefinitionChoice,
} from "@/domains/agent-configuration/types";
import {
  ManagementError,
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";

function BindingForm({
  workspace,
  definitions,
  value,
  close,
  saved,
}: {
  workspace: string;
  definitions: DefinitionChoice[];
  value?: Binding;
  close(): void;
  saved(): void;
}) {
  const { t } = useTranslation();
  const [id, setID] = useState(
    value?.definition_id ?? definitions[0]?.definition_id ?? "",
  );
  const [version, setVersion] = useState(
    value?.definition_version ?? definitions[0]?.version ?? 1,
  );
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const [credential, setCredential] = useState("");
  const [clear, setClear] = useState(false);
  const [header, setHeader] = useState(value?.credential_header ?? "X-API-Key");
  const [tools, setTools] = useState(
    (value?.allowed_tools ?? ["*"]).join("\n"),
  );
  const [prompts, setPrompts] = useState(
    (value?.allowed_prompts ?? ["*"]).join("\n"),
  );
  const [resources, setResources] = useState(
    (value?.allowed_resources ?? ["*"]).join("\n"),
  );
  const [definition, setDefinition] = useState<DefinitionChoice>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    setDefinition(undefined);
    if (!id) return;
    void managementRequest<DefinitionChoice>(
      `/api/v1/mcp-definitions/${encodeURIComponent(id)}?version=${version}`,
    )
      .then((data) => {
        if (active) {
          setDefinition(data);
          if (data.authentication_type === "api_key")
            setHeader((current) =>
              !current || current === "Authorization" ? "X-API-Key" : current,
            );
        }
      })
      .catch((cause: unknown) => {
        if (active) setError(cause);
      });
    return () => {
      active = false;
    };
  }, [id, version]);
  const current = definitions.find((item) => item.definition_id === id);
  const versions = [...new Set([version, current?.version ?? version])].sort(
    (a, b) => b - a,
  );
  const isStatic =
    definition?.authentication_type === "bearer" ||
    definition?.authentication_type === "api_key";
  const split = (input: string) =>
    input
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter(Boolean);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) close();
      }}
    >
      <DialogContent
        closeLabel={t("catalog.close")}
        className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"
      >
        <DialogHeader>
          <DialogTitle>
            {t(
              value ? "configuration.editBinding" : "configuration.addBinding",
            )}
          </DialogTitle>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={async (event) => {
            event.preventDefault();
            setBusy(true);
            setError(undefined);
            try {
              await managementWrite(
                bindingsPath(workspace, value?.binding_id),
                value ? "PUT" : "POST",
                {
                  definition_id: id,
                  definition_version: version,
                  enabled,
                  expected_version: value?.version ?? 0,
                  credential_header:
                    definition?.authentication_type === "api_key" ? header : "",
                  ...(isStatic && (credential || clear)
                    ? { credential: clear ? "" : credential }
                    : {}),
                  allowed_methods: value?.allowed_methods ?? ["*"],
                  allowed_tools: split(tools),
                  allowed_prompts: split(prompts),
                  allowed_resources: split(resources),
                },
              );
              setCredential("");
              saved();
              close();
            } catch (cause) {
              setError(cause);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label className="grid gap-1.5 text-sm">
            MCP
            <select
              aria-label={t("configuration.chooseMCP")}
              className="rounded-lg border bg-white px-3 py-2"
              value={id}
              disabled={!!value}
              onChange={(event) => {
                setID(event.target.value);
                setVersion(
                  definitions.find(
                    (item) => item.definition_id === event.target.value,
                  )?.version ?? 1,
                );
                setCredential("");
                setClear(false);
              }}
            >
              {value && !current && (
                <option value={value.definition_id}>
                  {value.definition_id}
                </option>
              )}
              {definitions.map((item) => (
                <option key={item.definition_id} value={item.definition_id}>
                  {item.service_name}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1.5 text-sm">
            {t("configuration.definitionVersion")}
            <select
              className="rounded-lg border bg-white px-3 py-2"
              value={version}
              onChange={(event) => {
                setVersion(Number(event.target.value));
                setCredential("");
                setClear(false);
              }}
            >
              {versions.map((item) => (
                <option key={item} value={item}>
                  {t("configuration.version", { version: item })}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
            />
            {t("configuration.bindingEnabled")}
          </label>
          {isStatic && (
            <>
              {definition?.authentication_type === "api_key" && (
                <label className="grid gap-1.5 text-sm">
                  {t("configuration.credentialHeader")}
                  <input
                    className="rounded-lg border px-3 py-2"
                    value={header}
                    onChange={(event) => setHeader(event.target.value)}
                    required
                  />
                </label>
              )}
              <label className="grid gap-1.5 text-sm">
                {t("configuration.credential")}
                <input
                  type="password"
                  autoComplete="new-password"
                  className="rounded-lg border px-3 py-2"
                  value={credential}
                  disabled={clear}
                  onChange={(event) => setCredential(event.target.value)}
                  placeholder={t(
                    value?.credential_configured
                      ? "configuration.keepCredential"
                      : "configuration.optional",
                  )}
                />
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={clear}
                  onChange={(event) => {
                    setClear(event.target.checked);
                    setCredential("");
                  }}
                />
                {t("configuration.clearCredential")}
              </label>
            </>
          )}
          {definition?.authentication_type === "oauth" && (
            <p className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
              {t("configuration.oauthAfterSave")}
            </p>
          )}
          <details className="rounded-lg border p-3">
            <summary className="cursor-pointer text-sm font-medium">
              {t("configuration.accessScope")}
            </summary>
            <p className="my-3 text-xs text-stone-500">
              {t("configuration.scopeHint")}
            </p>
            {[
              { key: "tools", value: tools, set: setTools },
              { key: "prompts", value: prompts, set: setPrompts },
              { key: "resources", value: resources, set: setResources },
            ].map((item) => (
              <label key={item.key} className="mb-3 grid gap-1 text-xs">
                {t(`configuration.scope_${item.key}`)}
                <textarea
                  className="min-h-16 rounded border p-2 font-mono"
                  value={item.value}
                  onChange={(event) => item.set(event.target.value)}
                />
              </label>
            ))}
          </details>
          <p className="text-xs leading-5 text-stone-500">
            {t("configuration.bindingCandidate")}
          </p>
          {!!error && (
            <p role="alert" className="text-sm text-red-700">
              {t(
                error instanceof ManagementError && error.status === 409
                  ? "configuration.conflict"
                  : "configuration.failed",
              )}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={close}
            >
              {t("configuration.cancel")}
            </Button>
            <Button disabled={busy || !definition || !id}>
              {t(busy ? "configuration.saving" : "configuration.saveBinding")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
export function BindingManager({
  workspace,
  definitions,
  canManage,
  changed,
}: {
  workspace: string;
  definitions: DefinitionChoice[];
  canManage: boolean;
  changed(): void;
}) {
  const { t } = useTranslation();
  const [bindings, setBindings] = useState<Binding[]>([]);
  const [edit, setEdit] = useState<Binding | null | undefined>();
  const [error, setError] = useState<unknown>();
  const [loading, setLoading] = useState(true);
  const [connection, setConnection] = useState<Binding>();
  const popup = useRef<Window | null>(null);
  const expectedOrigin = useRef("");
  const [disconnect, setDisconnect] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    try {
      setBindings(await listBindings(workspace));
      setError(undefined);
    } catch (cause) {
      setError(cause);
    } finally {
      setLoading(false);
    }
  }, [workspace]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    const listener = (event: MessageEvent) => {
      if (
        event.origin !== window.location.origin ||
        event.source !== popup.current ||
        !event.data ||
        typeof event.data !== "object" ||
        event.data.type !== "mcp-oauth-complete" ||
        event.data.server_origin !== expectedOrigin.current
      )
        return;
      popup.current = null;
      if (connection)
        void managementRequest<Binding>(
          bindingsPath(workspace, connection.binding_id),
        )
          .then(setConnection)
          .catch(setError);
    };
    window.addEventListener("message", listener);
    return () => {
      window.removeEventListener("message", listener);
    };
  }, [workspace, connection]);
  useEffect(
    () => () => {
      popup.current?.close();
      popup.current = null;
    },
    [],
  );
  function connected(value: Binding) {
    setConnection(value);
    expectedOrigin.current = value.oauth_authorize_url
      ? (new URL(
          value.oauth_authorize_url,
          window.location.origin,
        ).searchParams.get("server_origin") ?? "")
      : "";
  }
  const available = definitions.filter(
    (item) =>
      item.transport !== "platform" &&
      item.authentication_type !== "session_context" &&
      item.status === "enabled" &&
      !bindings.some((binding) => binding.definition_id === item.definition_id),
  );
  return (
    <section className="space-y-3 border-t pt-5">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="font-semibold">{t("configuration.connections")}</h3>
          <p className="mt-1 text-xs leading-5 text-stone-500">
            {t("configuration.connectionHint")}
          </p>
        </div>
        {canManage && (
          <Button
            size="sm"
            variant="outline"
            disabled={!available.length}
            onClick={() => setEdit(null)}
          >
            {t("configuration.addBinding")}
          </Button>
        )}
      </div>
      {loading && (
        <p className="text-sm text-stone-500">{t("configuration.loading")}</p>
      )}
      {!!error && (
        <p role="alert" className="text-sm text-red-700">
          {t("configuration.failed")}{" "}
          <button
            type="button"
            className="underline"
            onClick={() => void load()}
          >
            {t("configuration.reload")}
          </button>
        </p>
      )}
      {!loading && !bindings.length && (
        <p className="rounded-lg border border-dashed p-4 text-sm text-stone-500">
          {t("configuration.noBindings")}
        </p>
      )}
      {bindings.map((binding) => (
        <div
          key={binding.binding_id}
          className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-white p-3"
        >
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">
              {definitions.find(
                (item) => item.definition_id === binding.definition_id,
              )?.service_name ?? binding.definition_id}
            </p>
            <p className="mt-1 text-xs text-stone-500">
              {t(
                binding.enabled
                  ? "configuration.enabled"
                  : "configuration.disabled",
              )}{" "}
              · {t("configuration.version", { version: binding.version })}
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="ghost"
              onClick={async () => {
                try {
                  connected(
                    await managementRequest<Binding>(
                      bindingsPath(workspace, binding.binding_id),
                    ),
                  );
                } catch (cause) {
                  setError(cause);
                }
              }}
            >
              {t("configuration.connection")}
            </Button>
            {canManage && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setEdit(binding)}
              >
                {t("configuration.edit")}
              </Button>
            )}
          </div>
        </div>
      ))}
      {edit !== undefined && (
        <BindingForm
          workspace={workspace}
          definitions={edit ? definitions : available}
          value={edit ?? undefined}
          close={() => setEdit(undefined)}
          saved={() => {
            void load();
            changed();
          }}
        />
      )}
      {connection && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setConnection(undefined);
              setDisconnect(false);
            }
          }}
        >
          <DialogContent
            closeLabel={t("catalog.close")}
            className="sm:max-w-md"
          >
            <DialogHeader>
              <DialogTitle>
                {t("configuration.connection")}: {connection.definition_id}
              </DialogTitle>
            </DialogHeader>
            <p className="text-sm text-stone-600">
              {t(
                connection.oauth_authorize_url
                  ? connection.connection_status === "connected"
                    ? "configuration.oauthConnected"
                    : "configuration.needsReauth"
                  : connection.credential_configured
                    ? "configuration.credentialConfigured"
                    : connection.credential_header
                      ? "configuration.needsCredential"
                      : "configuration.noCredentialRequired",
              )}
            </p>
            {!!error && (
              <p role="alert" className="text-sm text-red-700">
                {t(
                  error instanceof Error && error.message === "popup_blocked"
                    ? "configuration.popupBlocked"
                    : "configuration.failed",
                )}
              </p>
            )}
            {connection.oauth_authorize_url && (
              <div className="mt-5 grid gap-3">
                <Button
                  onClick={() => {
                    setError(undefined);
                    if (popup.current && !popup.current.closed) {
                      popup.current.focus();
                      return;
                    }
                    const target = new URL(
                      connection.oauth_authorize_url ?? "",
                      window.location.origin,
                    );
                    if (target.origin !== window.location.origin) {
                      setError(new Error("invalid_oauth_origin"));
                      return;
                    }
                    popup.current = window.open(
                      target.href,
                      "media-mcp-oauth",
                      "popup,width=620,height=760",
                    );
                    if (!popup.current) setError(new Error("popup_blocked"));
                  }}
                >
                  {t(
                    connection.connection_status === "connected"
                      ? "configuration.reconnect"
                      : "configuration.connectOAuth",
                  )}
                </Button>
                <Button
                  variant="outline"
                  onClick={async () => {
                    try {
                      connected(
                        await managementRequest<Binding>(
                          bindingsPath(workspace, connection.binding_id),
                        ),
                      );
                    } catch (cause) {
                      setError(cause);
                    }
                  }}
                >
                  {t("configuration.refreshConnection")}
                </Button>
                {connection.connection_status === "connected" && (
                  <Button variant="outline" onClick={() => setDisconnect(true)}>
                    {t("configuration.disconnect")}
                  </Button>
                )}
                {disconnect && (
                  <div className="rounded border border-red-200 p-3">
                    <p className="mb-3 text-sm text-red-800">
                      {t("configuration.disconnectConfirm")}
                    </p>
                    <Button
                      variant="destructive"
                      onClick={async () => {
                        try {
                          await managementWrite(
                            `/api/mcp-oauth/${encodeURIComponent(expectedOrigin.current)}`,
                            "DELETE",
                            undefined,
                          );
                          connected(
                            await managementRequest<Binding>(
                              bindingsPath(workspace, connection.binding_id),
                            ),
                          );
                          setDisconnect(false);
                        } catch (cause) {
                          setError(cause);
                        }
                      }}
                    >
                      {t("configuration.confirmDisconnect")}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
