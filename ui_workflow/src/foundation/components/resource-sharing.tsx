import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

/** Command UI; never pretends session-local grants are a persisted grant list. */
export function ResourceSharing({
  organizationId,
  visibility,
  owner,
  onVisibility,
  onGrant,
  onRevoke,
  permissions = ["viewer", "editor"],
}: {
  organizationId?: string;
  visibility: string;
  owner: boolean;
  onVisibility: (value: "private" | "team" | "public") => Promise<unknown>;
  onGrant: (team: string, permission: "viewer" | "editor") => Promise<unknown>;
  onRevoke: (team: string) => Promise<unknown>;
  permissions?: readonly ("viewer" | "editor")[];
}) {
  const { t } = useTranslation();
  const [team, setTeam] = useState("");
  const [permission, setPermission] = useState<"viewer" | "editor">("viewer");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const teams = useQuery({
    queryKey: ["sharing-departments", organizationId],
    enabled: owner && Boolean(organizationId),
    queryFn: async () => {
      const response = await fetch("/api/teams", { credentials: "include" });
      if (!response.ok)
        throw new Error(`${t("sharing.departmentError")} (${response.status})`);
      return (await response.json()) as {
        teams?: {
          id: string;
          organization_id: string;
          name: string;
          status?: string;
        }[];
      };
    },
  });
  const choices = (teams.data?.teams ?? []).filter(
    (department) =>
      department.organization_id === organizationId &&
      (!department.status || department.status === "active"),
  );
  async function execute(action: () => Promise<unknown>) {
    setPending(true);
    setError("");
    setSaved(false);
    try {
      await action();
      setSaved(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("sharing.failed"));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="space-y-4" aria-label={t("sharing.title")}>
      <h2 className="font-semibold">{t("sharing.title")}</h2>
      <select
        aria-label={t("templateFields.visibility")}
        disabled={!owner || pending}
        value={visibility}
        className="h-9 w-full rounded-lg border bg-background px-3"
        onChange={(event) =>
          void execute(() =>
            onVisibility(event.target.value as "private" | "team" | "public"),
          )
        }
      >
        {(["private", "team", "public"] as const).map((value) => (
          <option key={value} value={value}>
            {t(`common.visibility.${value}`)}
          </option>
        ))}
      </select>
      {!owner ? (
        <p className="text-xs text-muted-foreground">
          {t("sharing.ownerOnly")}
        </p>
      ) : (
        <>
          <p className="text-xs leading-5 text-muted-foreground">
            {t("sharing.queryGap")}
          </p>
          {teams.isError ? (
            <div role="alert" className="space-y-2 text-xs text-destructive">
              <p>{teams.error.message}</p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void teams.refetch()}
              >
                {t("v12.retry")}
              </Button>
            </div>
          ) : null}
          <select
            aria-label={t("sharing.department")}
            className="h-9 w-full rounded-lg border bg-background px-3"
            disabled={pending || teams.isLoading}
            value={team}
            onChange={(event) => setTeam(event.target.value)}
          >
            <option value="">{t("sharing.chooseDepartment")}</option>
            {choices.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
          <select
            aria-label={t("sharing.permission")}
            className="h-9 w-full rounded-lg border bg-background px-3"
            disabled={pending}
            value={permission}
            onChange={(event) =>
              setPermission(event.target.value as "viewer" | "editor")
            }
          >
            {permissions.map((value) => (
              <option key={value} value={value}>
                {t(`sharing.${value}`)}
              </option>
            ))}
          </select>
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={
                pending ||
                !choices.some((item) => item.id === team) ||
                !permissions.includes(permission)
              }
              onClick={() => void execute(() => onGrant(team, permission))}
            >
              {t("sharing.grant")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={pending || !choices.some((item) => item.id === team)}
              onClick={() => void execute(() => onRevoke(team))}
            >
              {t("sharing.revoke")}
            </Button>
          </div>
        </>
      )}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {saved ? (
        <p role="status" className="text-xs text-success">
          {t("sharing.saved")}
        </p>
      ) : null}
    </section>
  );
}
