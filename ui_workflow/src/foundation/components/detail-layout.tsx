import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import i18next from "i18next";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/foundation/lib/utils";

export function DetailPage({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "h-full overflow-auto bg-gradient-to-b from-muted/20 via-background to-background",
        className,
      )}
    >
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 p-4 sm:p-6 lg:p-8">
        {children}
      </div>
    </div>
  );
}

export function DetailSection({
  title,
  description,
  icon: Icon,
  children,
  className,
}: {
  title: string;
  description?: string;
  icon?: LucideIcon;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn("border-border/70 bg-card/85 shadow-sm", className)}>
      <CardHeader className="pb-4">
        <div className="flex items-start gap-3">
          {Icon ? (
            <div className="rounded-xl bg-primary/10 p-2 text-primary">
              <Icon className="h-4 w-4" />
            </div>
          ) : null}
          <div>
            <CardTitle className="text-base">{title}</CardTitle>
            {description ? (
              <CardDescription className="mt-1">{description}</CardDescription>
            ) : null}
          </div>
        </div>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

export function DetailField({
  label,
  children,
  className,
}: {
  label: string;
  children?: ReactNode;
  className?: string;
}) {
  const { t } = useTranslation();
  const empty = children === undefined || children === null || children === "";
  return (
    <div
      className={cn("min-w-0 rounded-xl bg-muted/35 px-3.5 py-3", className)}
    >
      <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1.5 break-words text-sm leading-6 text-foreground">
        {empty ? (
          <span className="text-muted-foreground/70">
            {t("detailCommon.notFilled")}
          </span>
        ) : (
          children
        )}
      </dd>
    </div>
  );
}

export function DetailFieldGrid({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <dl className={cn("grid gap-3 sm:grid-cols-2 xl:grid-cols-3", className)}>
      {children}
    </dl>
  );
}

export function humanizeDetailKey(key: string): string {
  return key
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function DetailValue({ value }: { value: unknown }) {
  const { t } = useTranslation();
  if (value === null || value === undefined || value === "")
    return (
      <span className="text-muted-foreground/70">
        {t("detailCommon.notFilled")}
      </span>
    );
  if (typeof value === "boolean")
    return (
      <span className="inline-flex rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
        {t(value ? "detailCommon.yes" : "detailCommon.no")}
      </span>
    );
  if (Array.isArray(value)) {
    if (!value.length)
      return (
        <span className="text-muted-foreground/70">
          {t("detailCommon.empty")}
        </span>
      );
    if (value.every((item) => typeof item !== "object" || item === null))
      return (
        <div className="flex flex-wrap gap-1.5">
          {value.map((item, index) => (
            <span
              key={`${String(item)}-${index}`}
              className="rounded-full border border-border/80 bg-background px-2.5 py-1 text-xs"
            >
              {String(item)}
            </span>
          ))}
        </div>
      );
    return (
      <div className="grid gap-2">
        {value.map((item, index) => (
          <div
            key={index}
            className="rounded-lg border border-border/60 bg-background/70 p-3"
          >
            <DetailValue value={item} />
          </div>
        ))}
      </div>
    );
  }
  if (typeof value === "object")
    return (
      <StructuredDetails value={value as Record<string, unknown>} compact />
    );
  return <span className="whitespace-pre-wrap">{String(value)}</span>;
}

export function StructuredDetails({
  value,
  labels = {},
  compact = false,
}: {
  value: Record<string, unknown>;
  labels?: Record<string, string>;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const entries = Object.entries(value).filter(
    ([, item]) =>
      item !== null &&
      item !== undefined &&
      item !== "" &&
      (!Array.isArray(item) || item.length > 0),
  );
  if (!entries.length)
    return (
      <p className="text-sm text-muted-foreground">
        {t("detailCommon.noMetadata")}
      </p>
    );
  return (
    <dl
      className={cn("grid gap-3", compact ? "grid-cols-1" : "sm:grid-cols-2")}
    >
      {entries.map(([key, item]) => (
        <div
          key={key}
          className={cn(
            "min-w-0",
            !compact && "rounded-xl border border-border/60 bg-muted/20 p-3.5",
          )}
        >
          <dt className="text-xs font-medium text-muted-foreground">
            {labels[key] ?? humanizeDetailKey(key)}
          </dt>
          <dd className="mt-1.5 break-words text-sm leading-6">
            <DetailValue value={item} />
          </dd>
        </div>
      ))}
    </dl>
  );
}

export function formatDetailDate(
  value: string | number | undefined,
  locale = i18next.language || "zh-CN",
): string {
  if (value === undefined || value === "") return "-";
  try {
    const raw = BigInt(String(value));
    const milliseconds = raw > 9_999_999_999_999n ? raw / 1_000_000n : raw;
    const date = new Date(Number(milliseconds));
    if (!Number.isNaN(date.getTime()))
      return new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
  } catch {
    const date = new Date(value);
    if (!Number.isNaN(date.getTime()))
      return new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
  }
  return String(value);
}
