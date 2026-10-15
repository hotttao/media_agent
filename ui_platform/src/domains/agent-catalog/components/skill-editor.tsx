import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import {
  base64Text,
  bytesToBase64,
  catalogPath,
  downloadBase64,
  downloadSkill,
  saveResource,
  skillFile,
  skillFiles,
  textToBase64,
  versions,
} from "@/domains/agent-catalog/lib/catalog-client";
import type { CatalogResource } from "@/domains/agent-catalog/types";
import { managementWrite } from "@/foundation/lib/management-http";
import {
  CatalogError,
  CatalogModal,
  ConfirmAction,
  Field,
  inputClass,
} from "./shared";

function GitSourceEditor({
  resource,
  saved,
  close,
}: {
  resource: CatalogResource;
  saved(value: CatalogResource): void;
  close(): void;
}) {
  const { t } = useTranslation();
  const [url, setURL] = useState(resource.source?.url ?? "");
  const [branch, setBranch] = useState(resource.source?.branch ?? "");
  const [subpath, setSubpath] = useState(resource.source?.subpath ?? "");
  const [token, setToken] = useState("");
  const [clear, setClear] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  return (
    <CatalogModal
      title={t("catalog.gitSource")}
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
            const source = {
              url,
              branch,
              subpath,
              ...(token || clear ? { access_token: clear ? "" : token } : {}),
            };
            const value = await managementWrite<CatalogResource>(
              `${catalogPath("skills", resource.id)}/source`,
              "PUT",
              { expected_version: resource.version, source },
            );
            setToken("");
            saved(value);
            close();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Field label={t("catalog.repository")}>
          <input
            className={inputClass}
            value={url}
            onChange={(event) => setURL(event.target.value)}
            required
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("catalog.branch")}>
            <input
              className={inputClass}
              value={branch}
              onChange={(event) => setBranch(event.target.value)}
            />
          </Field>
          <Field label={t("catalog.subpath")}>
            <input
              className={inputClass}
              value={subpath}
              onChange={(event) => setSubpath(event.target.value)}
            />
          </Field>
        </div>
        <Field label={t("catalog.gitToken")}>
          <input
            className={inputClass}
            type="password"
            autoComplete="new-password"
            value={token}
            disabled={clear}
            onChange={(event) => setToken(event.target.value)}
            placeholder={t(
              resource.source?.credential_configured
                ? "catalog.leaveSecret"
                : "catalog.optional",
            )}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={clear}
            onChange={(event) => {
              setClear(event.target.checked);
              setToken("");
            }}
          />
          {t("catalog.clearCredential")}
        </label>
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
export function SkillEditor({
  resource,
  saved,
}: {
  resource: CatalogResource;
  saved(value: CatalogResource): void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(resource.name);
  const [description, setDescription] = useState(resource.description ?? "");
  const [version, setVersion] = useState(0);
  const [selectedPath, setSelectedPath] = useState("SKILL.md");
  const [path, setPath] = useState("");
  const [content, setContent] = useState("");
  const [executable, setExecutable] = useState(false);
  const [base, setBase] = useState({ content: "", executable: false });
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [showSource, setShowSource] = useState(false);
  useEffect(() => {
    setName(resource.name);
    setDescription(resource.description ?? "");
  }, [resource.name, resource.description]);
  const loadFiles = useCallback(
    () => skillFiles(resource.id, version),
    [resource.id, version],
  );
  const files = useCatalogLoad(loadFiles);
  const loadVersions = useCallback(
    () => versions("skills", resource.id),
    [resource.id],
  );
  const history = useCatalogLoad(loadVersions);
  const loadFile = useCallback(
    () =>
      selectedPath
        ? skillFile(resource.id, selectedPath, version)
        : Promise.resolve({
            path: "",
            content: "",
            binary: false,
            executable: false,
          }),
    [resource.id, selectedPath, version],
  );
  const file = useCatalogLoad(loadFile);
  useEffect(() => {
    if (file.data) {
      setPath(file.data.path);
      const value = file.data.binary ? "" : base64Text(file.data.content ?? "");
      setContent(value);
      setExecutable(!!file.data.executable);
      setBase({ content: value, executable: !!file.data.executable });
    }
  }, [file.data]);
  const editable =
    resource.my_permission !== "viewer" && resource.status !== "archived";
  const dirty =
    content !== base.content ||
    executable !== base.executable ||
    path !== selectedPath;
  function navigate(next: () => void) {
    if (dirty && !window.confirm(t("catalog.discardEdits"))) return;
    next();
  }
  async function action(suffix: string, body: Record<string, unknown> = {}) {
    setError(undefined);
    setBusy(true);
    try {
      const value = await managementWrite<CatalogResource>(
        `${catalogPath("skills", resource.id)}${suffix}`,
        "POST",
        { ...body, expected_version: resource.version },
      );
      saved(value);
      files.reload();
      file.reload();
      history.reload();
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="rounded bg-amber-100 px-2 py-1 text-xs text-amber-900">
          {t("catalog.fixedUpdates")}
        </span>
        <select
          aria-label={t("catalog.history")}
          className={`${inputClass} max-w-52`}
          value={version}
          disabled={busy}
          onChange={(event) =>
            navigate(() => setVersion(Number(event.target.value)))
          }
        >
          <option value={0}>{t("catalog.draft")}</option>
          {history.data?.map((item) => (
            <option key={item.version} value={item.version}>
              {t("catalog.publishedVersion", { version: item.version })}
            </option>
          ))}
        </select>
      </div>
      <CatalogError error={error} />
      <CatalogError error={history.error} retry={history.reload} />
      <div className="flex flex-wrap gap-2">
        {version > 0 && (
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              void downloadSkill(resource.id, version).catch(setError)
            }
          >
            {t("catalog.download")}
          </Button>
        )}
        {editable && !version && (
          <>
            <ConfirmAction
              title={t("catalog.publish")}
              description={t("catalog.publishConfirm", { name: resource.name })}
              disabled={busy || dirty}
              onError={setError}
              action={() => action("/publish")}
            />
            {(resource.published_version ?? 0) > 0 && resource.has_draft && (
              <ConfirmAction
                title={t("catalog.discardDraft")}
                description={t("catalog.discardDraftConfirm")}
                disabled={busy}
                onError={setError}
                action={() => action("/draft:discard")}
              />
            )}
            <label className="inline-flex cursor-pointer items-center rounded-md border bg-white px-3 py-1.5 text-xs font-medium">
              {t("catalog.uploadArchive")}
              <input
                type="file"
                className="sr-only"
                accept=".zip,.tar.gz,.tgz"
                disabled={busy}
                onChange={async (event) => {
                  const uploaded = event.target.files?.[0];
                  event.target.value = "";
                  if (!uploaded) return;
                  if (uploaded.size > 8 * 1024 * 1024) {
                    setError(new Error("archive_too_large"));
                    return;
                  }
                  if (dirty && !window.confirm(t("catalog.discardEdits")))
                    return;
                  setBusy(true);
                  setError(undefined);
                  try {
                    const value = await managementWrite<CatalogResource>(
                      `${catalogPath("skills", resource.id)}/draft`,
                      "PUT",
                      {
                        expected_version: resource.version,
                        content: bytesToBase64(
                          new Uint8Array(await uploaded.arrayBuffer()),
                        ),
                      },
                    );
                    saved(value);
                    files.reload();
                    file.reload();
                  } catch (cause) {
                    setError(cause);
                  } finally {
                    setBusy(false);
                  }
                }}
              />
            </label>
          </>
        )}
        {resource.source_kind === "git" && editable && (
          <ConfirmAction
            title={t("catalog.syncGit")}
            description={t("catalog.syncConfirm")}
            disabled={busy}
            onError={setError}
            action={() => action("/git:sync")}
          />
        )}{" "}
        {resource.my_permission === "owner" && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setShowSource(true)}
          >
            {t("catalog.gitSource")}
          </Button>
        )}
      </div>
      <div className="grid min-h-[380px] gap-3 rounded-lg border bg-white md:grid-cols-[180px_minmax(0,1fr)]">
        <div className="border-b p-3 md:border-b-0 md:border-r">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-stone-500">
              {t("catalog.files")}
            </h3>
            {editable && !version && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => navigate(() => setSelectedPath(""))}
              >
                {t("catalog.addFile")}
              </Button>
            )}
          </div>
          <CatalogError error={files.error} retry={files.reload} />
          {files.loading && (
            <p className="text-sm text-stone-500">{t("catalog.loading")}</p>
          )}
          <div className="grid gap-1">
            {files.data?.map((item) => (
              <button
                type="button"
                key={item.path}
                className={`break-all rounded px-2 py-1.5 text-left text-xs ${item.path === selectedPath ? "bg-amber-100 font-medium" : "hover:bg-stone-100"}`}
                onClick={() => navigate(() => setSelectedPath(item.path))}
              >
                {item.path}
              </button>
            ))}
          </div>
        </div>
        <div className="grid content-start gap-3 p-3">
          <CatalogError error={file.error} retry={file.reload} />
          {file.loading ? (
            <p className="text-sm text-stone-500">{t("catalog.loading")}</p>
          ) : (
            <>
              <Field label={t("catalog.filePath")}>
                <input
                  className={inputClass}
                  value={path}
                  onChange={(event) => setPath(event.target.value)}
                  readOnly={!editable || !!version || !!selectedPath}
                  placeholder="SKILL.md"
                />
              </Field>
              {file.data?.binary ? (
                <>
                  <p className="text-sm text-stone-500">
                    {t("catalog.binaryFile")}
                  </p>
                  <Button
                    variant="outline"
                    onClick={() =>
                      downloadBase64(
                        file.data?.content ?? "",
                        path.split("/").pop() ?? path,
                      )
                    }
                  >
                    {t("catalog.download")}
                  </Button>
                </>
              ) : (
                <textarea
                  aria-label={t("catalog.fileContent")}
                  className={`${inputClass} min-h-[300px] resize-y font-mono leading-6`}
                  value={content}
                  readOnly={!editable || !!version}
                  onChange={(event) => setContent(event.target.value)}
                />
              )}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="flex items-center gap-2 text-xs">
                  <input
                    type="checkbox"
                    checked={executable}
                    disabled={!editable || !!version || !!file.data?.binary}
                    onChange={(event) => setExecutable(event.target.checked)}
                  />
                  {t("catalog.executable")}
                </label>
                {editable && !version && (
                  <div className="flex gap-2">
                    {selectedPath && (
                      <ConfirmAction
                        title={t("catalog.deleteFile")}
                        description={t("catalog.deleteFileConfirm", {
                          name: path,
                        })}
                        disabled={busy}
                        onError={setError}
                        action={async () => {
                          const value = await managementWrite<CatalogResource>(
                            `${catalogPath("skills", resource.id)}/file?${new URLSearchParams({ path: selectedPath, expected_version: String(resource.version) })}`,
                            "DELETE",
                            undefined,
                          );
                          saved(value);
                          setSelectedPath("");
                          files.reload();
                        }}
                      />
                    )}
                    {!file.data?.binary && (
                      <Button
                        size="sm"
                        disabled={busy || !path || !dirty}
                        onClick={async () => {
                          setBusy(true);
                          setError(undefined);
                          try {
                            const value =
                              await managementWrite<CatalogResource>(
                                `${catalogPath("skills", resource.id)}/file`,
                                "PUT",
                                {
                                  expected_version: resource.version,
                                  path,
                                  content: textToBase64(content),
                                  executable,
                                },
                              );
                            saved(value);
                            setBase({ content, executable });
                            setSelectedPath(path);
                            files.reload();
                          } catch (cause) {
                            setError(cause);
                          } finally {
                            setBusy(false);
                          }
                        }}
                      >
                        {t(busy ? "catalog.saving" : "catalog.saveFile")}
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
      <p className="text-xs leading-5 text-stone-500">
        {t("catalog.skillRules")}
      </p>
      {editable && (
        <form
          className="grid gap-3 border-t pt-4"
          onSubmit={async (event) => {
            event.preventDefault();
            setBusy(true);
            try {
              saved(
                await saveResource("skills", resource, { name, description }),
              );
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
              required
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field label={t("catalog.description")}>
            <textarea
              className={inputClass}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </Field>
          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={
                busy ||
                (name === resource.name &&
                  description === (resource.description ?? ""))
              }
            >
              {t("catalog.saveDetails")}
            </Button>
          </div>
        </form>
      )}
      {showSource && (
        <GitSourceEditor
          resource={resource}
          saved={saved}
          close={() => setShowSource(false)}
        />
      )}
    </div>
  );
}
