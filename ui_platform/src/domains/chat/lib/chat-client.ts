import type {
  ChatMessage,
  ChatSession,
  UniversalEvent,
} from "@/domains/chat/types";

async function checked(response: Response): Promise<Response> {
  if (response.status === 401) {
    window.location.href = "/login";
    throw new Error("Session expired");
  }
  if (!response.ok) {
    const body = await response
      .json()
      .catch(() => ({ error: "Request failed" }));
    throw new Error(
      typeof body.error === "string" ? body.error : "Request failed",
    );
  }
  return response;
}

export async function getDefaultSession(): Promise<ChatSession> {
  const response = await checked(
    await fetch("/api/chat/session", { credentials: "include" }),
  );
  return response.json();
}

export async function listSessions(
  workspaceId: string,
): Promise<ChatSession[]> {
  const response = await checked(
    await fetch(
      `/api/chat/sessions?workspace_id=${encodeURIComponent(workspaceId)}`,
      { credentials: "include" },
    ),
  );
  const body = (await response.json()) as { sessions?: ChatSession[] };
  return body.sessions ?? [];
}

export async function createSession(
  workspaceId: string,
  title: string,
): Promise<ChatSession> {
  const response = await checked(
    await fetch("/api/chat/sessions", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ workspace_id: workspaceId, title }),
    }),
  );
  return response.json();
}

export async function listMessages(sessionId: string): Promise<ChatMessage[]> {
  const response = await checked(
    await fetch(`/api/chat/sessions/${sessionId}/messages`, {
      credentials: "include",
    }),
  );
  const body = (await response.json()) as { messages?: ChatMessage[] };
  return body.messages ?? [];
}

export async function runTurn(
  sessionId: string,
  message: string,
): Promise<{ turn_id: string; status: string }> {
  return checked(
    await fetch(`/api/chat/sessions/${sessionId}/turns`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ message, idempotency_key: createClientId() }),
    }),
  ).then((response) => response.json());
}

export async function subscribeSessionEvents(
  sessionId: string,
  afterSequence: number,
  signal: AbortSignal,
  onEvent: (event: UniversalEvent, sequence: number) => void,
): Promise<number> {
  const response = await checked(
    await fetch(
      `/api/chat/sessions/${sessionId}/events?after_sequence=${afterSequence}`,
      {
        credentials: "include",
        headers: { Accept: "text/event-stream" },
        signal,
      },
    ),
  );
  if (!response.body) throw new Error("Event subscription is unavailable");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const frames = buffer.split(/\r?\n\r?\n/);
    buffer = frames.pop() ?? "";
    for (const frame of frames) {
      const decoded = decodeSessionEventFrame(frame, afterSequence);
      if (!decoded) continue;
      afterSequence = decoded.cursor;
      onEvent(decoded.event, decoded.cursor);
    }
    if (done) break;
  }
  return afterSequence;
}

export function decodeSessionEventFrame(
  frame: string,
  cursor: number,
): { event: UniversalEvent; cursor: number } | null {
  const lines = frame.split(/\r?\n/);
  const eventName = lines
    .find((line) => line.startsWith("event:"))
    ?.slice(6)
    .trim();
  if (eventName && eventName !== "message") return null;
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart())
    .join("\n");
  if (!data) return null;
  const id = lines
    .find((line) => line.startsWith("id:"))
    ?.slice(3)
    .trim();
  if (id === undefined || id === "") {
    return { event: JSON.parse(data) as UniversalEvent, cursor };
  }
  const sequence = Number(id);
  if (!Number.isSafeInteger(sequence) || sequence <= cursor) return null;
  return { event: JSON.parse(data) as UniversalEvent, cursor: sequence };
}

export function createClientId(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  const bytes = new Uint8Array(16);
  if (typeof globalThis.crypto?.getRandomValues === "function") {
    globalThis.crypto.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function itemText(item: UniversalEvent["item"]): string {
  return (item?.content ?? [])
    .map((part) => part.text ?? part.output ?? "")
    .join("");
}

export function storedContentText(contentJson: string): string {
  try {
    const parsed = JSON.parse(contentJson);
    const content = Array.isArray(parsed) ? parsed : parsed.content;
    return Array.isArray(content)
      ? content.map((part) => part.text ?? part.output ?? "").join("")
      : contentJson;
  } catch {
    return contentJson;
  }
}
