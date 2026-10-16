import { BookOpen, ChevronLeft, FileText, Plug, Puzzle } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { MCPCatalog } from "@/domains/agent-catalog/components/mcp-catalog";
import { ResourceLibrary } from "@/domains/agent-catalog/components/resource-library";
import {
  CatalogError,
  inputClass,
} from "@/domains/agent-catalog/components/shared";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import type { CatalogKind } from "@/domains/agent-catalog/types";
import { SignOutButton } from "@/foundation/components/sign-out-button";
import {
  listMyTeams,
  listOrganizations,
} from "@/foundation/lib/hierarchy-client";
import { useAuth } from "@/foundation/providers/auth-provider";
export function AgentLibraryPage() {
  const { t, i18n } = useTranslation();
  const { user } = useAuth();
  const { organizationId } = useParams();
  const navigate = useNavigate();
  const [kind, setKind] = useState<CatalogKind>("prompts");
  const load = useCallback(async () => {
    const [organizations, teams] = await Promise.all([
      listOrganizations(),
      listMyTeams(),
    ]);
    return { organizations, teams };
  }, []);
  const request = useCatalogLoad(load);
  const organization =
    request.data?.organizations.find((item) => item.id === organizationId) ??
    (!organizationId ? request.data?.organizations[0] : undefined);
  return (
    <main className="agent-product flex h-[100dvh] flex-col bg-[#f5f4f0] text-stone-900">
      <header className="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-3 border-b border-stone-200 bg-white px-4 py-2 sm:px-6">
        <div className="flex min-w-0 max-w-full flex-wrap items-center gap-3">
          <Link
            to="/"
            className="flex items-center gap-1 text-sm text-stone-500 hover:text-stone-900"
          >
            <ChevronLeft size={16} />
            {t("catalog.backToWorkspace")}
          </Link>
          <span className="h-5 border-l" />
          <BookOpen size={18} />
          <span className="font-semibold">{t("catalog.library")}</span>
        </div>
        <div className="flex min-w-0 max-w-full flex-wrap items-center gap-3">
          <select
            aria-label={t("catalog.organization")}
            className={`${inputClass} max-w-40 sm:max-w-52`}
            value={organization?.id ?? ""}
            onChange={(event) =>
              navigate(`/library/${encodeURIComponent(event.target.value)}`)
            }
          >
            <option value="" disabled>
              {t("catalog.chooseOrganization")}
            </option>
            {request.data?.organizations.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              void i18n.changeLanguage(
                i18n.language.startsWith("zh") ? "en-US" : "zh-CN",
              )
            }
          >
            {i18n.language.startsWith("zh") ? "English" : "中文"}
          </Button>
          <Link to="/model-connections" className="text-sm">
            {t("modelConnections.title")}
          </Link>
          <SignOutButton />
        </div>
      </header>
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <nav
          aria-label={t("catalog.library")}
          className="flex shrink-0 gap-1 border-b p-3 md:w-52 md:flex-col md:border-b-0 md:border-r md:p-4"
        >
          {(
            [
              { id: "prompts", Icon: FileText },
              { id: "skills", Icon: Puzzle },
              { id: "mcp", Icon: Plug },
            ] as const
          ).map(({ id, Icon }) => (
            <button
              key={id}
              type="button"
              className={`flex min-w-0 flex-1 items-center gap-2 rounded-lg px-3 py-2.5 text-sm md:flex-none ${kind === id ? "bg-stone-900 font-semibold text-white" : "text-stone-600 hover:bg-white"}`}
              onClick={() => setKind(id)}
            >
              <Icon size={16} />
              {t(`catalog.${id}`)}
            </button>
          ))}
        </nav>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          {request.loading ? (
            <p className="p-8 text-sm text-stone-500">{t("catalog.loading")}</p>
          ) : request.error ? (
            <div className="p-8">
              <CatalogError error={request.error} retry={request.reload} />
            </div>
          ) : organization ? (
            kind === "mcp" ? (
              <MCPCatalog
                key={organization.id}
                organization={organization.id}
                admin={organization.my_role === "admin"}
                platformAdmin={user?.role === "admin"}
              />
            ) : (
              <ResourceLibrary
                key={`${organization.id}:${kind}`}
                kind={kind}
                organization={organization.id}
                organizationName={organization.name}
                teams={request.data?.teams ?? []}
              />
            )
          ) : (
            <div className="grid flex-1 place-items-center p-8 text-center text-sm text-stone-500">
              {t(
                organizationId ? "catalog.forbidden" : "catalog.noOrganization",
              )}
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
