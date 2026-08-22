import { useState } from "react";
import { Button } from "@/components/ui/button";
import type { AgentRequest } from "@/domains/chat/types";

function textValue(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function optionsOf(request: AgentRequest): Array<{ id: string; label: string }> {
  const options = request.payload.options;
  if (!Array.isArray(options)) return [];
  return options.flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const item = value as Record<string, unknown>;
    const id = item.option_id ?? item.optionId;
    return typeof id === "string"
      ? [{ id, label: textValue(item.name ?? item.label ?? item.description, id) }]
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
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const label = textValue(
    request.payload.question ?? request.payload.prompt,
    request.request_type === "permission"
      ? "The agent requests permission"
      : "The agent needs your input",
  );
  async function resolve(response: Record<string, unknown>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await onResolve(response);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
      setBusy(false);
    }
  }
  return (
    <section
      aria-label={`${request.request_type} request`}
      className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm"
    >
      <p className="font-medium text-amber-950">{label}</p>
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
              {option.label}
            </Button>
          ))}
        </div>
      )}
      {request.request_type === "confirm" && (
        <div className="mt-3 flex gap-2">
          <Button type="button" size="sm" disabled={busy} onClick={() => void resolve({ confirmed: true })}>
            Confirm
          </Button>
          <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => void resolve({ confirmed: false })}>
            Cancel
          </Button>
        </div>
      )}
      {request.request_type !== "permission" && request.request_type !== "confirm" && (
        <form
          className="mt-3 flex gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (value.trim()) void resolve({ value: value.trim() });
          }}
        >
          <input
            aria-label="Agent input"
            value={value}
            disabled={busy}
            onChange={(event) => setValue(event.target.value)}
            className="min-w-0 flex-1 rounded-xl border border-amber-300 bg-white px-3 py-2"
          />
          <Button type="submit" size="sm" disabled={busy || !value.trim()}>
            Send
          </Button>
        </form>
      )}
      {error && <p className="mt-2 text-xs text-red-700">{error}</p>}
    </section>
  );
}
