import { useTranslation } from "react-i18next";
import type { InvocationExpression } from "@/domains/templates/types";

function ExpressionView({ expression }: { expression: InvocationExpression }) {
  const { t } = useTranslation();
  if (expression.op === "list")
    return (
      <ol className="list-decimal space-y-2 pl-5">
        {(expression.items ?? []).map((item, index) => (
          <li key={index}>
            <ExpressionView expression={item} />
          </li>
        ))}
      </ol>
    );
  return (
    <span>
      {expression.op === "asset_ref"
        ? `${t("mapping.asset_ref")} · ${expression.slot || "—"}`
        : t(`mapping.${expression.op}`)}
    </span>
  );
}

export function InvocationMappingView({
  value,
}: {
  value: Record<string, InvocationExpression>;
}) {
  const { t } = useTranslation();
  return (
    <section aria-label={t("mapping.title")} className="space-y-4">
      <h2 className="font-semibold">{t("mapping.title")}</h2>
      {Object.entries(value).map(([name, expression]) => (
        <div key={name} className="rounded-xl border bg-background p-4 text-sm">
          <h3 className="mb-2 font-semibold">{name}</h3>
          <ExpressionView expression={expression} />
        </div>
      ))}
    </section>
  );
}
