import type { BrowserFlow } from "@/foundation/lib/ory-browser";

export type OryAuthFlow = Extract<BrowserFlow, "login" | "registration">;

export interface OryMessage {
  id: number;
  text: string;
  type: "error" | "info" | "success";
}

export interface OryInputAttributes {
  name: string;
  type: string;
  value?: string;
  required?: boolean;
  disabled?: boolean;
  autocomplete?: string;
  node_type: "input";
}

export interface OryUiNode {
  type: string;
  group: string;
  attributes: OryInputAttributes;
  messages: OryMessage[];
  meta: {
    label?: {
      text: string;
    };
  };
}

export interface OryBrowserFlow {
  id: string;
  state: string;
  return_to?: string;
  ui: {
    action: string;
    method: string;
    nodes: OryUiNode[];
    messages?: OryMessage[];
  };
}

export class OryFlowError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "OryFlowError";
    this.status = status;
  }
}

function errorMessage(payload: unknown, fallback: string): string {
  if (!payload || typeof payload !== "object") return fallback;
  const record = payload as Record<string, unknown>;
  const error = record.error;
  if (error && typeof error === "object") {
    const reason = (error as Record<string, unknown>).reason;
    if (typeof reason === "string") return reason;
  }
  return fallback;
}

export function validateFlowAction(
  action: string,
  kind: OryAuthFlow,
  origin = window.location.origin,
): string {
  const url = new URL(action, origin);
  if (
    url.origin !== new URL(origin).origin ||
    url.pathname !== `/self-service/${kind}`
  ) {
    throw new Error("Kratos returned an invalid authentication action");
  }
  return url.toString();
}

export function safeReturnURL(
  returnTo: string | undefined,
  origin = window.location.origin,
): string {
  if (!returnTo) return new URL("/", origin).toString();
  const url = new URL(returnTo, origin);
  return url.origin === new URL(origin).origin
    ? url.toString()
    : new URL("/", origin).toString();
}

export async function loadBrowserFlow(
  kind: OryAuthFlow,
  id: string,
  request: typeof fetch = fetch,
): Promise<OryBrowserFlow> {
  const response = await request(
    `/self-service/${kind}/flows?id=${encodeURIComponent(id)}`,
    {
      credentials: "include",
      headers: { Accept: "application/json" },
    },
  );
  const payload = (await response.json().catch(() => null)) as unknown;
  if (!response.ok) {
    throw new OryFlowError(
      errorMessage(payload, `Unable to load ${kind} flow`),
      response.status,
    );
  }
  return payload as OryBrowserFlow;
}

export type SubmitFlowResult =
  | { status: "flow"; flow: OryBrowserFlow }
  | { status: "complete" };

export async function submitBrowserFlow(
  kind: OryAuthFlow,
  flow: OryBrowserFlow,
  values: Record<string, string>,
  request: typeof fetch = fetch,
  origin = window.location.origin,
): Promise<SubmitFlowResult> {
  const response = await request(
    validateFlowAction(flow.ui.action, kind, origin),
    {
      method: flow.ui.method,
      credentials: "include",
      redirect: "follow",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(values),
    },
  );
  const payload = (await response.json().catch(() => null)) as unknown;
  if (payload && typeof payload === "object" && "ui" in payload) {
    return { status: "flow", flow: payload as OryBrowserFlow };
  }
  if (response.ok) return { status: "complete" };
  throw new OryFlowError(
    errorMessage(payload, `Unable to submit ${kind} flow`),
    response.status,
  );
}
