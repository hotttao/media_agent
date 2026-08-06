import type { PropsWithChildren } from "react";
import { useTranslation } from "react-i18next";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { navigationItems } from "@/foundation/lib/navigation";

export function ApplicationShell({ children }: PropsWithChildren) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const activePath =
    navigationItems.find((item) => location.pathname.startsWith(item.path))
      ?.path ?? "";

  return (
    <div className="flex h-full min-h-0 bg-background text-foreground">
      <aside className="hidden w-60 shrink-0 border-r bg-card p-4 md:flex md:flex-col">
        <div className="mb-6">
          <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t("app.eyebrow")}
          </p>
          <h1 className="text-lg font-semibold">{t("app.title")}</h1>
        </div>
        <nav aria-label={t("navigation.label")} className="space-y-1">
          {navigationItems.map((item) => (
            <NavLink
              className={({ isActive }) =>
                `block rounded-md px-3 py-2 text-sm transition-colors ${
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                }`
              }
              key={item.id}
              to={item.path}
            >
              {t(item.labelKey)}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="border-b bg-card px-4 py-3 md:hidden">
          <label className="sr-only" htmlFor="materials-mobile-navigation">
            {t("navigation.label")}
          </label>
          <select
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            id="materials-mobile-navigation"
            onChange={(event) => navigate(event.target.value)}
            value={activePath}
          >
            <option disabled value="">
              {t("navigation.choose")}
            </option>
            {navigationItems.map((item) => (
              <option key={item.id} value={item.path}>
                {t(item.labelKey)}
              </option>
            ))}
          </select>
        </header>
        <main className="min-h-0 flex-1 overflow-auto p-4 sm:p-6">
          {children}
        </main>
      </div>
    </div>
  );
}
