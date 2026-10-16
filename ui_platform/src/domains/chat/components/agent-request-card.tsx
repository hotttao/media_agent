import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { AgentRequest } from "@/domains/chat/types";

function textValue(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function optionsOf(
  request: AgentRequest,
): Array<{ id: string; label: string }> {
  const options = request.payload.options;
  if (!Array.isArray(options)) return [];
  return options.flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const item = value as Record<string, unknown>;
    const id = item.option_id ?? item.optionId;
    return typeof id === "string"
      ? [
          {
            id,
            label: textValue(item.name ?? item.label ?? item.description, id),
          },
        ]
      : [];
  });
}

export function AgentRequestCard({
  request,
  onResolve,
}: {
  request: AgentRequest;
  onResolve(response: Record<string, unknown>): Promise<void>;
}) {
  const { t } = useTranslation();
  const permissionLabels: Record<string, string> = {
    allow_once: t("agentRequest.allowOnce"),
    allow_always: t("agentRequest.allowAlways"),
    reject_once: t("agentRequest.rejectOnce"),
    reject_always: t("agentRequest.rejectAlways"),
  };
  const tool = request.payload.tool_call ?? request.payload.toolCall;
  const toolTitle =
    tool && typeof tool === "object"
      ? (tool as Record<string, unknown>).title
      : undefined;
  const raw =
    tool && typeof tool === "object"
      ? (tool as Record<string, unknown>).raw_input
      : undefined;
  const commandValue =
    raw && typeof raw === "object"
      ? (raw as Record<string, unknown>).command
      : undefined;
  const command =
    typeof commandValue === "string"
      ? commandValue
      : Array.isArray(commandValue) &&
          commandValue.every((part) => typeof part === "string")
        ? JSON.stringify(commandValue, null, 2)
        : "";
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const label = textValue(
    request.payload.question ?? request.payload.prompt ?? toolTitle,
    request.request_type === "permission"
      ? t("agentRequest.permission")
      : t("agentRequest.input"),
  );
  async function resolve(response: Record<string, unknown>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await onResolve(response);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("agentRequest.failed"),
      );
      setBusy(false);
    }
  }
  return (
    <section
      aria-label={`${request.request_type} request`}
      className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm"
    >
      <p className="font-medium text-amber-950">{label}</p>
      {request.request_type === "permission" && command && (
        <div role="group" aria-label={t("agentRequest.command")}>
          <pre className="mt-3 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-amber-200 bg-white p-3 text-xs">
            {command}
          </pre>
        </div>
      )}
      {request.request_type === "permission" && (
        <div className="mt-3 flex flex-wrap gap-2">
          {optionsOf(request).map((option) => (
            <Button
              key={option.id}
              type="button"
              size="sm"
              disabled={busy}
              onClick={() => void resolve({ option_id: option.id })}
            >
              {permissionLabels[option.label] ?? option.label}
            </Button>
          ))}
        </div>
      )}
      {request.request_type === "confirm" && (
        <div className="mt-3 flex gap-2">
          <Button
            type="button"
            size="sm"
            disabled={busy}
            onClick={() => void resolve({ confirmed: true })}
          >
            {t("agentRequest.confirm")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => void resolve({ confirmed: false })}
          >
            {t("agentRequest.cancel")}
          </Button>
        </div>
      )}
      {request.request_type !== "permission" &&
        request.request_type !== "confirm" && (
          <form
            className="mt-3 flex gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              if (value.trim()) void resolve({ value: value.trim() });
            }}
          >
            <input
              aria-label={t("agentRequest.inputLabel")}
              value={value}
              disabled={busy}
              onChange={(event) => setValue(event.target.value)}
              className="min-w-0 flex-1 rounded-xl border border-amber-300 bg-white px-3 py-2"
            />
            <Button type="submit" size="sm" disabled={busy || !value.trim()}>
              {t("agentRequest.send")}
            </Button>
          </form>
        )}
      {error && <p className="mt-2 text-xs text-red-700">{error}</p>}
    </section>
  );
}
