import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  resourceKind,
  selectedPrompt,
  selectionBody,
} from "@/domains/agent-configuration/lib/selection";
import type {
  MetadataSelection,
  PromptSelection,
  ResourceChoice,
} from "@/domains/agent-configuration/types";
import {
  ManagementError,
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";

function VersionPicker({
  choice,
  value,
  workspace,
  disabled,
  change,
}: {
  choice: ResourceChoice;
  value: number;
  workspace: string;
  disabled: boolean;
  change(value: number): void;
}) {
  const { t } = useTranslation();
  const [versions, setVersions] = useState<number[]>([]);
  const [error, setError] = useState(false);
  const load = useCallback(async () => {
    try {
      const path =
        choice.kind === "mcp"
          ? `/api/v1/workspaces/${encodeURIComponent(workspace)}/mcp-bindings/${encodeURIComponent(choice.id)}/versions`
          : `/api/v1/${choice.kind}/${encodeURIComponent(choice.id)}/versions`;
      const body = await managementRequest<{
        versions?: { version: number }[];
        bindings?: { version: number }[];
      }>(path);
      setVersions(
        (body.versions ?? body.bindings ?? []).map((item) => item.version),
      );
      setError(false);
    } catch {
      setError(true);
    }
  }, [workspace, choice.kind, choice.id]);
  const options = [...new Set([value, choice.version, ...versions])]
    .filter((item) => item > 0)
    .sort((a, b) => b - a);
  return (
    <span className="inline-grid gap-1">
      <select
        aria-label={t("configuration.fixedVersion", { name: choice.name })}
        className="rounded border bg-white px-2 py-1 text-xs"
        value={value}
        disabled={disabled}
        onFocus={() => void load()}
        onChange={(event) => change(Number(event.target.value))}
      >
        {options.map((version) => (
          <option key={version} value={version}>
            {t("configuration.version", { version })}
            {version === choice.version
              ? ` · ${t("configuration.latest")}`
              : ""}
          </option>
        ))}
      </select>
      {!!error && (
        <button
          type="button"
          onClick={() => void load()}
          className="text-left text-xs text-red-700"
        >
          {t("configuration.retryVersions")}
        </button>
      )}
    </span>
  );
}
export function MetadataEditor({
  selection,
  choices,
  workspace,
  canManage,
  saved,
  openLibrary,
}: {
  selection: MetadataSelection;
  choices: ResourceChoice[];
  workspace: string;
  canManage: boolean;
  saved(): Promise<void>;
  openLibrary(): void;
}) {
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState<PromptSelection>(() =>
    selectedPrompt(selection),
  );
  const [refs, setRefs] = useState(selection.resources ?? []);
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [success, setSuccess] = useState(false);
  const pending = useRef<{ fingerprint: string; key: string }>();
  const initial = JSON.stringify(
    selectionBody(selectedPrompt(selection), selection.resources ?? []),
  );
  const body = selectionBody(prompt, refs);
  const dirty = JSON.stringify(body) !== initial;
  const prompts = choices.filter((item) => item.kind === "prompts");
  const selectedPromptChoice = prompts.find(
    (item) => item.id === prompt.resource_id,
  );
  const allChoices = [...choices.filter((item) => item.kind !== "prompts")];
  for (const ref of refs) {
    if (
      ref.kind !== "media_control.skill" &&
      ref.kind !== "media_plugin.mcp_binding"
    )
      continue;
    if (
      !allChoices.some(
        (item) => item.id === ref.id && resourceKind(item.kind) === ref.kind,
      )
    )
      allChoices.push({
        id: ref.id,
        kind: ref.kind === "media_control.skill" ? "skills" : "mcp",
        name: t("configuration.unavailableSelection"),
        version: ref.version,
        disabled: true,
      });
  }
  function choose(choice: ResourceChoice, checked: boolean) {
    const kind = resourceKind(choice.kind);
    setRefs((values) =>
      checked
        ? [
            ...values.filter(
              (ref) => ref.id !== choice.id || ref.kind !== kind,
            ),
            { kind, id: choice.id, version: choice.version },
          ]
        : values.filter((ref) => ref.id !== choice.id || ref.kind !== kind),
    );
    setSuccess(false);
  }
  const errorStatus = error instanceof ManagementError ? error.status : 0;
  return (
    <section className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-base font-semibold">
            {t("configuration.title")}
          </h3>
          <p className="mt-1 text-xs leading-5 text-stone-500">
            {t("configuration.pinnedHint")}
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={openLibrary}>
          {t("configuration.openLibrary")}
        </Button>
      </div>
      {!canManage && (
        <p className="rounded bg-stone-100 p-3 text-sm text-stone-600">
          {t("configuration.readOnly")}
        </p>
      )}
      <fieldset disabled={!canManage || busy} className="space-y-4">
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">{t("configuration.prompt")}</span>
          <select
            className="rounded-lg border bg-white px-3 py-2"
            value={prompt.mode}
            onChange={(event) => {
              const mode = event.target.value as PromptSelection["mode"];
              setPrompt(
                mode === "library"
                  ? {
                      mode,
                      resource_id: prompts[0]?.id ?? "",
                      resource_version: prompts[0]?.version ?? 0,
                    }
                  : mode === "custom"
                    ? { mode, content: "" }
                    : { mode },
              );
              setSuccess(false);
            }}
          >
            <option value="none">{t("configuration.promptNone")}</option>
            <option value="custom">{t("configuration.promptCustom")}</option>
            <option value="library">{t("configuration.promptLibrary")}</option>
          </select>
        </label>
        {prompt.mode === "custom" && (
          <textarea
            aria-label={t("configuration.customBody")}
            className="min-h-40 w-full rounded-lg border bg-white p-3 text-sm leading-6"
            value={prompt.content ?? ""}
            onChange={(event) =>
              setPrompt({ mode: "custom", content: event.target.value })
            }
          />
        )}{" "}
        {prompt.mode === "library" && (
          <div className="flex flex-wrap gap-2">
            <select
              aria-label={t("configuration.choosePrompt")}
              className="min-w-0 flex-1 rounded-lg border bg-white px-3 py-2 text-sm"
              value={prompt.resource_id ?? ""}
              onChange={(event) => {
                const choice = prompts.find(
                  (item) => item.id === event.target.value,
                );
                setPrompt({
                  mode: "library",
                  resource_id: choice?.id ?? "",
                  resource_version: choice?.version ?? 0,
                });
              }}
            >
              <option value="" disabled>
                {t("configuration.choosePrompt")}
              </option>
              {!selectedPromptChoice && prompt.resource_id && (
                <option value={prompt.resource_id}>
                  {t("configuration.unavailableSelection")}
                </option>
              )}
              {prompts.map((choice) => (
                <option key={choice.id} value={choice.id}>
                  {choice.name}
                </option>
              ))}
            </select>
            {prompt.resource_id && (
              <VersionPicker
                choice={
                  selectedPromptChoice ?? {
                    id: prompt.resource_id,
                    name: t("configuration.prompt"),
                    kind: "prompts",
                    version: prompt.resource_version ?? 1,
                  }
                }
                value={prompt.resource_version ?? 1}
                workspace={workspace}
                disabled={!canManage}
                change={(version) =>
                  setPrompt({ ...prompt, resource_version: version })
                }
              />
            )}
          </div>
        )}
        {(["skills", "mcp"] as const).map((kind) => (
          <div key={kind} className="space-y-2">
            <h4 className="text-sm font-medium">
              {t(`configuration.${kind}`)}
            </h4>
            {!allChoices.some((item) => item.kind === kind) && (
              <p className="text-xs text-stone-500">
                {t("configuration.noChoices")}
              </p>
            )}
            {allChoices
              .filter((item) => item.kind === kind)
              .map((choice) => {
                const selected = refs.find(
                  (item) =>
                    item.id === choice.id && item.kind === resourceKind(kind),
                );
                return (
                  <div
                    key={choice.id}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-white p-3"
                  >
                    <label className="flex min-w-0 flex-1 items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={!!selected}
                        disabled={
                          (choice.disabled && !selected) ||
                          (choice.version < 1 && !selected)
                        }
                        onChange={(event) =>
                          choose(choice, event.target.checked)
                        }
                      />
                      <span className="break-words">
                        {choice.name}
                        {choice.disabled && (
                          <span className="ml-2 text-xs text-amber-800">
                            {t("configuration.notSelectable")}
                          </span>
                        )}
                      </span>
                    </label>
                    {selected && (
                      <VersionPicker
                        choice={choice}
                        value={selected.version}
                        workspace={workspace}
                        disabled={!canManage || choice.disabled === true}
                        change={(version) =>
                          setRefs((values) =>
                            values.map((item) =>
                              item.id === choice.id &&
                              item.kind === resourceKind(kind)
                                ? { ...item, version, checksum: undefined }
                                : item,
                            ),
                          )
                        }
                      />
                    )}
                  </div>
                );
              })}
          </div>
        ))}
      </fieldset>
      {!!error && (
        <div
          role="alert"
          className="rounded-lg bg-red-50 p-3 text-sm text-red-700"
        >
          {t(
            errorStatus === 409
              ? "configuration.conflict"
              : errorStatus === 403
                ? "configuration.forbidden"
                : "configuration.failed",
          )}
        </div>
      )}
      {success && (
        <p
          role="status"
          className="rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800"
        >
          {t("configuration.saved")}
        </p>
      )}
      {canManage && (
        <Button
          disabled={
            busy ||
            !dirty ||
            (prompt.mode === "library" &&
              (!prompt.resource_id || !prompt.resource_version))
          }
          onClick={() => setReview(true)}
        >
          {t("configuration.review")}
        </Button>
      )}
      {review && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !busy) setReview(false);
          }}
        >
          <DialogContent
            closeLabel={t("configuration.cancel")}
            className="max-h-[90dvh] overflow-y-auto sm:max-w-lg"
          >
            <DialogHeader>
              <DialogTitle>{t("configuration.confirmTitle")}</DialogTitle>
            </DialogHeader>
            <p className="text-sm leading-6 text-stone-600">
              {t("configuration.confirmHint")}
            </p>
            <dl className="my-4 grid gap-3 rounded-lg bg-stone-50 p-4 text-sm">
              <div>
                <dt className="text-xs text-stone-500">
                  {t("configuration.prompt")}
                </dt>
                <dd>
                  {t(`configuration.mode_${prompt.mode}`)}
                  {prompt.mode === "library"
                    ? ` · ${selectedPromptChoice?.name ?? prompt.resource_id} · v${prompt.resource_version}`
                    : ""}
                </dd>
              </div>
              {body.resources
                .filter(
                  (ref) =>
                    ref.kind === "media_control.skill" ||
                    ref.kind === "media_plugin.mcp_binding",
                )
                .map((ref) => (
                  <div key={`${ref.kind}:${ref.id}`}>
                    <dt className="text-xs text-stone-500">
                      {ref.kind === "media_control.skill"
                        ? t("configuration.skills")
                        : "MCP"}
                    </dt>
                    <dd>
                      {allChoices.find((choice) => choice.id === ref.id)
                        ?.name ?? ref.id}{" "}
                      · v{ref.version}
                    </dd>
                  </div>
                ))}
            </dl>
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setReview(false)}
              >
                {t("configuration.cancel")}
              </Button>
              <Button
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  setError(undefined);
                  try {
                    const fingerprint = JSON.stringify({
                      agent_config_id: selection.agent_config_id,
                      expected_version: selection.version,
                      ...body,
                    });
                    if (pending.current?.fingerprint !== fingerprint)
                      pending.current = {
                        fingerprint,
                        key: crypto.randomUUID(),
                      };
                    await managementWrite(
                      `/api/v1/workspaces/${encodeURIComponent(workspace)}/agent-metadata:confirm`,
                      "POST",
                      {
                        agent_config_id: selection.agent_config_id,
                        expected_version: selection.version,
                        idempotency_key: pending.current.key,
                        ...body,
                      },
                    );
                    setReview(false);
                    setSuccess(true);
                    pending.current = undefined;
                    await saved();
                  } catch (cause) {
                    setError(cause);
                    setReview(false);
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {t(busy ? "configuration.saving" : "configuration.confirm")}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
