import { type FormEvent, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  createWorkspace,
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

export function WorkspaceFormDialog({
  mode,
  workspace,
  open,
  onClose,
  onSaved,
}: {
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
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setName(workspace?.name ?? "");
      setAgentType(workspace?.agent_type ?? "codex");
      setApiKey("");
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
  if (!open) return null;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (
      !name.trim() ||
      saving ||
      (mode === "create" && (!organizationID || !teamID))
    )
      return;
    setSaving(true);
    setError("");
    try {
      const input = {
        name: name.trim(),
        agent_type: agentType,
        api_key: apiKey.trim(),
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
        className="w-full max-w-md space-y-4 rounded-3xl bg-white p-6 shadow-2xl"
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
            onChange={(event) => setAgentType(event.target.value as AgentType)}
            className="w-full rounded-xl border bg-white px-3 py-2.5 text-sm"
          >
            {AGENT_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
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
