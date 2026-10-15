import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  bytesToBase64,
  catalogPath,
} from "@/domains/agent-catalog/lib/catalog-client";
import type { CatalogResource } from "@/domains/agent-catalog/types";
import { managementWrite } from "@/foundation/lib/management-http";
import { CatalogError, CatalogModal, Field, inputClass } from "./shared";
export function CreateResource({
  kind,
  organization,
  close,
  created,
}: {
  kind: "prompts" | "skills";
  organization: string;
  close(): void;
  created(value: CatalogResource): void;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState("online");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [content, setContent] = useState("");
  const [slug, setSlug] = useState("");
  const [archive, setArchive] = useState<File>();
  const [url, setURL] = useState("");
  const [branch, setBranch] = useState("");
  const [subpath, setSubpath] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  return (
    <CatalogModal
      title={t(kind === "prompts" ? "catalog.newPrompt" : "catalog.newSkill")}
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
            const body: Record<string, unknown> = {
              organization_id: organization,
              name,
              visibility: "private",
              description,
            };
            if (kind === "prompts") body.content = content;
            else if (mode === "online") body.slug = slug;
            else if (mode === "upload") {
              if (!archive || archive.size > 8 * 1024 * 1024)
                throw new Error("invalid_archive");
              body.content = bytesToBase64(
                new Uint8Array(await archive.arrayBuffer()),
              );
            } else
              body.git = {
                url,
                branch,
                subpath,
                ...(token ? { access_token: token } : {}),
              };
            const value = await managementWrite<CatalogResource>(
              `${catalogPath(kind)}${kind === "skills" && mode === "git" ? "/import-git" : ""}`,
              "POST",
              body,
            );
            setToken("");
            created(value);
            close();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        {kind === "skills" && (
          <Field label={t("catalog.createMethod")}>
            <select
              className={inputClass}
              value={mode}
              onChange={(event) => {
                setMode(event.target.value);
                setToken("");
              }}
            >
              <option value="online">{t("catalog.online")}</option>
              <option value="upload">{t("catalog.uploadArchive")}</option>
              <option value="git">{t("catalog.importGit")}</option>
            </select>
          </Field>
        )}
        <Field label={t("catalog.name")}>
          <input
            className={inputClass}
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={200}
          />
        </Field>
        {kind === "prompts" ? (
          <Field label={t("catalog.promptBody")}>
            <textarea
              className={`${inputClass} min-h-48 font-mono`}
              value={content}
              onChange={(event) => setContent(event.target.value)}
            />
          </Field>
        ) : (
          <>
            <Field label={t("catalog.description")}>
              <textarea
                className={inputClass}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                required={mode === "online"}
              />
            </Field>
            {mode === "online" && (
              <Field label={t("catalog.skillSlug")}>
                <input
                  className={inputClass}
                  value={slug}
                  onChange={(event) => setSlug(event.target.value)}
                  pattern="[a-z0-9]+(-[a-z0-9]+)*"
                  maxLength={64}
                  required
                  placeholder="video-writing"
                />
              </Field>
            )}
            {mode === "upload" && (
              <Field label={t("catalog.archive")}>
                <input
                  type="file"
                  accept=".zip,.tar.gz,.tgz"
                  required
                  onChange={(event) => setArchive(event.target.files?.[0])}
                />
              </Field>
            )}
            {mode === "git" && (
              <>
                <Field label={t("catalog.repository")}>
                  <input
                    className={inputClass}
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                    required
                    placeholder="https://github.com/company/skills.git"
                  />
                </Field>
                <div className="grid gap-3 sm:grid-cols-2">
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
                    type="password"
                    autoComplete="new-password"
                    className={inputClass}
                    value={token}
                    onChange={(event) => setToken(event.target.value)}
                  />
                </Field>
              </>
            )}
          </>
        )}
        <p className="text-xs leading-5 text-stone-500">
          {t("catalog.createPrivate")}
          {kind === "skills" && ` ${t("catalog.skillRules")}`}
        </p>
        <CatalogError error={error} />
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={close}
          >
            {t("catalog.cancel")}
          </Button>
          <Button disabled={busy || !name.trim()}>
            {t(busy ? "catalog.saving" : "catalog.create")}
          </Button>
        </div>
      </form>
    </CatalogModal>
  );
}
