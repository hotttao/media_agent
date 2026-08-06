import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

interface ResourcePageScaffoldProps {
  resourceKey: string;
  mode: "list" | "create" | "edit" | "show";
  listPath: string;
  createPath?: string;
}

export function ResourcePageScaffold({
  resourceKey,
  mode,
  listPath,
  createPath,
}: ResourcePageScaffoldProps) {
  const { t } = useTranslation();

  return (
    <section
      className="mx-auto max-w-6xl"
      data-testid={`${resourceKey}-${mode}-page`}
    >
      <div className="flex flex-wrap items-start justify-between gap-4 border-b pb-5">
        <div>
          <p className="text-sm text-muted-foreground">
            {t(`pageModes.${mode}`)}
          </p>
          <h2 className="text-2xl font-semibold tracking-tight">
            {t(`resources.${resourceKey}.title`)}
          </h2>
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
            {t(`resources.${resourceKey}.description`)}
          </p>
        </div>
        {mode === "list" && createPath ? (
          <Link
            className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground"
            to={createPath}
          >
            {t("actions.create")}
          </Link>
        ) : null}
      </div>
      <div className="mt-6 rounded-lg border border-dashed bg-card p-10 text-center">
        <p className="font-medium">{t("architecture.title")}</p>
        <p className="mx-auto mt-2 max-w-xl text-sm text-muted-foreground">
          {t("architecture.description")}
        </p>
        {mode !== "list" ? (
          <Link
            className="mt-5 inline-block text-sm font-medium underline"
            to={listPath}
          >
            {t("actions.backToList")}
          </Link>
        ) : null}
      </div>
    </section>
  );
}
