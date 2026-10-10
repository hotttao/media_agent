import { useEffect, useId } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ListState } from "@/foundation/components/resource-list-layout";
import { getCapability, listCapabilities } from "../lib/api";
import type { Capability } from "../types";

export function CapabilitySelector({
  capabilityKey,
  version,
  onSelect,
  onReady,
}: {
  capabilityKey: string;
  version: number;
  onSelect: (contract: Capability) => void;
  onReady: (ready: boolean) => void;
}) {
  const { t } = useTranslation();
  const selectorId = useId();
  const catalog = useQuery({
    queryKey: ["capability-catalog"],
    queryFn: listCapabilities,
  });
  const selected = useQuery({
    queryKey: ["capability-version", capabilityKey, version],
    queryFn: () => getCapability(capabilityKey, version),
    enabled:
      Boolean(capabilityKey) && Number.isSafeInteger(version) && version > 0,
  });
  const valid =
    selected.isSuccess &&
    selected.data.key === capabilityKey &&
    selected.data.version === version;
  useEffect(() => onReady(valid), [valid, onReady, capabilityKey, version]);
  const ref = capabilityKey ? `${capabilityKey}@${version}` : "";
  return (
    <section className="space-y-3">
      <label htmlFor={selectorId} className="text-sm font-medium">
        {t("templateEditor.capability")}
      </label>
      <select
        id={selectorId}
        className="h-10 w-full rounded-lg border bg-background px-3 text-sm"
        value={ref}
        disabled={catalog.isLoading || catalog.isError}
        onChange={(event) => {
          const next = catalog.data?.find(
            (item) => `${item.key}@${item.version}` === event.target.value,
          );
          if (next) onSelect(next);
        }}
      >
        <option value="">{t("capability.choose")}</option>
        {ref &&
        !catalog.data?.some((item) => `${item.key}@${item.version}` === ref) ? (
          <option value={ref}>{ref}</option>
        ) : null}
        {(catalog.data ?? []).map((item) => (
          <option
            key={`${item.key}@${item.version}`}
            value={`${item.key}@${item.version}`}
          >
            {item.provider} · {t(`capability.method.${item.generation_method}`)}{" "}
            · {t(`capability.type.${item.generation_type}`)} · v{item.version} ·{" "}
            {t(`capability.${item.execution_target}`)}
          </option>
        ))}
      </select>
      {catalog.isLoading || (capabilityKey && selected.isLoading) ? (
        <ListState loading />
      ) : null}
      {catalog.isError ? (
        <ListState
          error={catalog.error.message}
          onRetry={() => void catalog.refetch()}
        />
      ) : null}
      {selected.isError ? (
        <ListState
          error={selected.error.message}
          onRetry={() => void selected.refetch()}
        />
      ) : null}
      {catalog.isSuccess && !catalog.data.length ? (
        <p className="text-xs text-muted-foreground">{t("capability.empty")}</p>
      ) : null}
      {valid ? (
        <div className="space-y-2 rounded-xl bg-muted/40 p-3 text-xs">
          <p>
            {selected.data.key}@{selected.data.version}
          </p>
          <p>
            {t("capability.executionTarget")} ·{" "}
            {t(`capability.${selected.data.execution_target}`)}
          </p>
          <p>
            {t("capability.output")} ·{" "}
            {t(`capability.media.${selected.data.output_media_type}`)}
          </p>
          <ul>
            {(selected.data.inputs ?? []).map((input) => (
              <li key={input.name}>
                {input.name} · {t(`capability.input.${input.type}`)}{" "}
                {input.required ? " *" : ""}
                {input.type === "resource_ref_list"
                  ? ` · ${input.min_items ?? 0}–${input.max_items || "∞"}`
                  : ""}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground">
            {t("capability.readOnlyHint")}
          </p>
        </div>
      ) : null}
    </section>
  );
}
