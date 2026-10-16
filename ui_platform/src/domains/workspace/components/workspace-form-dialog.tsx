import { type FormEvent, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  createWorkspace,
  listAvailableModelConnections,
  updateWorkspace,
} from "@/domains/workspace/lib/workspace-client";
import {
  AGENT_TYPE_OPTIONS,
  type AgentType,
  type Workspace,
} from "@/domains/workspace/types";
import {
  listMyTeams,
  listOrganizations,
} from "@/foundation/lib/hierarchy-client";
import type { Organization, Team } from "@/foundation/types/hierarchy";
import type { ModelConnectionChoice } from "@/foundation/types/model-connection";

export function WorkspaceFormDialog({
  mode,
  workspace,
  open,
  onClose,
  onSaved,
  modelConnectionsHref = "/settings/models",
}: {
  modelConnectionsHref?: string;
  mode: "create" | "edit";
  workspace?: Workspace;
  open: boolean;
  onClose(): void;
  onSaved(value: Workspace): void;
}) {
  const { t } = useTranslation();
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [organizationID, setOrganizationID] = useState("");
  const [teamID, setTeamID] = useState("");
  const [loadingHierarchy, setLoadingHierarchy] = useState(false);
  const [name, setName] = useState("");
  const [agentType, setAgentType] = useState<AgentType>("codex");
  const [apiKey, setApiKey] = useState("");
  const [modelProtocol, setModelProtocol] = useState("anthropic");
  const [modelName, setModelName] = useState("");
  const [modelBaseURL, setModelBaseURL] = useState("");
  const [authMode, setAuthMode] = useState("api_key");
  const [connectionID, setConnectionID] = useState("");
  const [connections, setConnections] = useState<ModelConnectionChoice[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setName(workspace?.name ?? "");
      setAgentType(workspace?.agent_type ?? "codex");
      setApiKey("");
      setModelProtocol(
        workspace?.model_protocol ||
          (workspace?.agent_type === "goose" ? "openai" : "anthropic"),
      );
      setModelName(workspace?.model_name ?? "");
      setModelBaseURL(workspace?.model_base_url ?? "");
      setAuthMode(workspace?.model_connection_id ? "connection" : "api_key");
      setConnectionID(workspace?.model_connection_id ?? "");
      setError("");
    }
  }, [open, workspace]);
  useEffect(() => {
    if (!open) {
      setApiKey("");
      return;
    }
    if (mode !== "create") return;
    let active = true;
    setLoadingHierarchy(true);
    setOrganizations([]);
    setTeams([]);
    setOrganizationID("");
    setTeamID("");
    void Promise.all([listOrganizations(), listMyTeams()])
      .then(([orgs, departments]) => {
        if (!active) return;
        setOrganizations(orgs);
        setTeams(departments);
        const org = orgs[0]?.id ?? "";
        setOrganizationID(org);
        setTeamID(
          departments.find((team) => team.organization_id === org)?.id ?? "",
        );
      })
      .catch((cause: unknown) => {
        if (active)
          setError(cause instanceof Error ? cause.message : "Request failed");
      })
      .finally(() => {
        if (active) setLoadingHierarchy(false);
      });
    return () => {
      active = false;
    };
  }, [open, mode]);
  useEffect(() => {
    if (!open) return;
    let active = true;
    listAvailableModelConnections()
      .then((v) => {
        if (active) setConnections(v.filter((c) => c.status === "active"));
      })
      .catch(() => {
        if (active) setConnections([]);
      });
    return () => {
      active = false;
    };
  }, [open]);
  if (!open) return null;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (
      !name.trim() ||
      saving ||
      (authMode === "connection" && !connectionID) ||
      (mode === "create" && (!organizationID || !teamID))
    )
      return;
    setSaving(true);
    setError("");
    try {
      const input = {
        name: name.trim(),
        agent_type: agentType,
        api_key: authMode === "api_key" ? apiKey.trim() : "",
        model_connection_id: authMode === "connection" ? connectionID : "",
        ...(agentType !== "codex"
          ? {
              model_protocol: modelProtocol,
              model_name: modelName.trim(),
              model_base_url: modelBaseURL.trim(),
            }
          : {}),
      };
      const value =
        mode === "create"
          ? await createWorkspace({
              ...input,
              organization_id: organizationID,
              team_id: teamID,
            })
          : await updateWorkspace(workspace?.id ?? "", input);
      setApiKey("");
      onSaved(value);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-50 grid place-items-center bg-slate-950/30 p-4 backdrop-blur-sm"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <form
        role="dialog"
        aria-modal="true"
        aria-label={mode === "create" ? "Create workspace" : "Edit workspace"}
        onSubmit={submit}
        className="max-h-[calc(100dvh-2rem)] w-full max-w-md space-y-4 overflow-y-auto rounded-3xl bg-white p-6 shadow-2xl"
      >
        <div>
          <h2 className="text-xl font-semibold">
            {mode === "create" ? "New workspace" : "Workspace settings"}
          </h2>
          <p className="mt-1 text-sm text-slate-500">
            Choose the agent configuration.
          </p>
        </div>
        <label className="block space-y-1.5">
          <span className="text-sm font-medium">Workspace name</span>
          <input
            aria-label={
              mode === "create" ? "Workspace name" : "Settings workspace name"
            }
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="w-full rounded-xl border px-3 py-2.5 text-sm"
          />
        </label>
        {mode === "create" && (
          <>
            <label className="block space-y-1.5">
              <span className="text-sm font-medium">
                {t("catalog.organization")}
              </span>
              <select
                aria-label={t("catalog.organization")}
                className="w-full rounded-xl border bg-white px-3 py-2.5 text-sm"
                value={organizationID}
                disabled={loadingHierarchy}
                onChange={(event) => {
                  const id = event.target.value;
                  setOrganizationID(id);
                  setTeamID(
                    teams.find((team) => team.organization_id === id)?.id ?? "",
                  );
                }}
              >
                <option value="">
                  {t(
                    loadingHierarchy
                      ? "catalog.loading"
                      : "catalog.chooseOrganization",
                  )}
                </option>
                {organizations.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="block space-y-1.5">
              <span className="text-sm font-medium">{t("catalog.team")}</span>
              <select
                aria-label={t("catalog.team")}
                className="w-full rounded-xl border bg-white px-3 py-2.5 text-sm"
                value={teamID}
                onChange={(event) => setTeamID(event.target.value)}
                disabled={loadingHierarchy || !organizationID}
              >
                <option value="">{t("catalog.chooseTeam")}</option>
                {teams
                  .filter((team) => team.organization_id === organizationID)
                  .map((team) => (
                    <option key={team.id} value={team.id}>
                      {team.name}
                    </option>
                  ))}
              </select>
            </label>
            {!loadingHierarchy &&
              (!organizations.length ||
                !teams.some(
                  (team) => team.organization_id === organizationID,
                )) && (
                <p className="text-sm text-amber-800">
                  {t("catalog.membershipRequired")}
                </p>
              )}
          </>
        )}
        <label className="block space-y-1.5">
          <span className="text-sm font-medium">Agent</span>
          <select
            aria-label="Agent"
            value={agentType}
            onChange={(event) => {
              setAgentType(event.target.value as AgentType);
              setAuthMode("api_key");
              setConnectionID("");
              setApiKey("");
              setModelProtocol(
                event.target.value === "goose" ? "openai" : "anthropic",
              );
              setModelName("");
              setModelBaseURL("");
            }}
            className="w-full rounded-xl border bg-white px-3 py-2.5 text-sm"
          >
            {AGENT_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        {agentType !== "codex" && (
          <fieldset className="space-y-4 rounded-xl border border-stone-200 p-4">
            <legend className="px-1 text-sm font-medium">
              {t("runtimeModels.title")}
            </legend>
            <label className="block space-y-1.5 text-sm">
              <span>{t("runtimeModels.protocol")}</span>
              <select
                aria-label={t("runtimeModels.protocol")}
                value={modelProtocol}
                onChange={(e) => {
                  setModelProtocol(e.target.value);
                  setApiKey("");
                }}
                className="w-full rounded-lg border bg-white px-3 py-2"
              >
                <option value="anthropic">
                  {t("runtimeModels.anthropic")}
                </option>
                {agentType === "goose" && (
                  <option value="openai">{t("runtimeModels.openai")}</option>
                )}
              </select>
            </label>
            <label className="block space-y-1.5 text-sm">
              <span>{t("runtimeModels.model")}</span>
              <input
                aria-label={t("runtimeModels.model")}
                value={modelName}
                maxLength={256}
                onChange={(e) => setModelName(e.target.value)}
                required={agentType === "goose"}
                className="w-full rounded-lg border px-3 py-2"
                placeholder={
                  agentType === "claude-code"
                    ? t("runtimeModels.defaultModel")
                    : t("runtimeModels.modelHint")
                }
              />
            </label>
            <label className="block space-y-1.5 text-sm">
              <span>{t("runtimeModels.baseURL")}</span>
              <input
                aria-label={t("runtimeModels.baseURL")}
                type="url"
                value={modelBaseURL}
                maxLength={2048}
                onChange={(e) => setModelBaseURL(e.target.value)}
                className="w-full rounded-lg border px-3 py-2"
                placeholder={t("runtimeModels.defaultEndpoint")}
              />
            </label>
            <p className="text-xs leading-5 text-stone-500">
              {t("runtimeModels.description")}
            </p>
          </fieldset>
        )}
        {agentType === "codex" && (
          <label className="block space-y-1.5">
            <span className="text-sm font-medium">
              {t("modelConnections.authentication")}
            </span>
            <select
              aria-label={t("modelConnections.authentication")}
              className="w-full rounded-xl border bg-white px-3 py-2.5 text-sm"
              value={authMode}
              onChange={(e) => {
                setAuthMode(e.target.value);
                setApiKey("");
              }}
            >
              <option value="api_key">API Key</option>
              <option value="connection">
                {t("modelConnections.personal")}
              </option>
            </select>
          </label>
        )}
        {agentType === "codex" && authMode === "connection" ? (
          <div className="space-y-2">
            <label className="block text-sm">
              {t("modelConnections.select")}
              <select
                className="mt-2 w-full rounded-xl border bg-white px-3 py-2.5"
                aria-label={t("modelConnections.select")}
                value={connectionID}
                onChange={(e) => setConnectionID(e.target.value)}
                required
              >
                <option value="">{t("modelConnections.choose")}</option>
                {connectionID &&
                  !connections.some((c) => c.id === connectionID) && (
                    <option value={connectionID}>
                      {t("modelConnections.configured")}
                    </option>
                  )}
                {connections.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
            <p className="text-xs text-slate-500">
              {t("modelConnections.workspaceAccess")}
            </p>
            <Link
              className="text-sm underline"
              to={modelConnectionsHref}
              onClick={onClose}
            >
              {t("modelConnections.manage")}
            </Link>
          </div>
        ) : (
          <label className="block space-y-1.5">
            <span className="text-sm font-medium">API key</span>
            <input
              aria-label={mode === "create" ? "API key" : "Settings API key"}
              type="password"
              autoComplete="off"
              value={apiKey}
              onChange={(event) => setApiKey(event.target.value)}
              placeholder={
                workspace?.api_key_configured
                  ? "Configured — leave blank to keep"
                  : "Optional"
              }
              className="w-full rounded-xl border px-3 py-2.5 text-sm"
            />
          </label>
        )}
        {error && (
          <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            disabled={
              !name.trim() ||
              (authMode === "connection" && !connectionID) ||
              saving ||
              (mode === "create" &&
                (!organizationID || !teamID || loadingHierarchy))
            }
          >
            {saving
              ? "Saving…"
              : mode === "create"
                ? "Create workspace"
                : "Save changes"}
          </Button>
        </div>
      </form>
    </div>
  );
}
