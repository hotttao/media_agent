import {
  Bot,
  ChevronDown,
  MessageSquare,
  Plus,
  Search,
  Send,
  Settings,
  Sparkles,
  Square,
  Users,
} from "lucide-react";
import {
  createContext,
  type FormEvent,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { MessageBubble } from "@/domains/chat/components/message-bubble";
import { AgentRequestCard } from "@/domains/chat/components/agent-request-card";
import {
  createClientId,
  createSession,
  interruptTurn,
  itemText,
  listMessages,
  listAgentRequests,
  listSessions,
  runTurn,
  resolveAgentRequest,
  subscribeSessionEvents,
} from "@/domains/chat/lib/chat-client";
import { isSessionCompatible } from "@/domains/chat/lib/session-compatibility";
import type {
  AgentRequest,
  ChatMessage,
  ChatSession,
  UniversalEvent,
} from "@/domains/chat/types";
import { WorkspaceFormDialog } from "@/domains/workspace/components/workspace-form-dialog";
import { listWorkspaces } from "@/domains/workspace/lib/workspace-client";
import { agentTypeLabel, type Workspace } from "@/domains/workspace/types";
import { useAppHeaderSlot } from "@/foundation/components/app-window";
import { SegmentedDock } from "@/foundation/components/segmented-dock";
import { WorkspaceLayout } from "@/foundation/components/workspace-layout";
import { api } from "@/foundation/lib/api-client";
import { useAuth } from "@/foundation/providers/auth-provider";
import { SlotProvider, useSlots } from "@/foundation/providers/slot-provider";
import { createAppRegistry } from "@/foundation/slots/app-registry";
import type { SlotConfig } from "@/foundation/slots/types";

interface WorkspaceRuntime {
  workspace: Workspace;
  sessions: ChatSession[];
  sessionId: string;
  messages: ChatMessage[];
  loading: boolean;
  streaming: boolean;
  interrupting: boolean;
  error: string;
  setError(value: string): void;
  selectSession(id: string): Promise<void>;
  addSession(): Promise<void>;
  send(text: string): Promise<void>;
  interrupt(): Promise<void>;
  requests: AgentRequest[];
  resolveRequest(
    request: AgentRequest,
    response: Record<string, unknown>,
  ): Promise<void>;
  openSettings(): void;
}

const RuntimeContext = createContext<WorkspaceRuntime | null>(null);
const SLOT_CONFIGS: SlotConfig[] = [
  { id: "slot-a", defaultOpened: ["sessions"] },
  { id: "slot-b", defaultOpened: ["workspace-settings"] },
  { id: "slot-c", defaultOpened: ["chat"] },
];
function useRuntime() {
  const value = useContext(RuntimeContext);
  if (!value) throw new Error("Workspace app requires runtime context");
  return value;
}

function HeaderPortal({ children }: { children: React.ReactNode }) {
  const target = useAppHeaderSlot();
  return target ? createPortal(children, target) : null;
}

function SessionsApp() {
  const { sessions, sessionId, selectSession, addSession } = useRuntime();
  const { ensureInstance } = useSlots();
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <HeaderPortal>
        <MessageSquare size={15} className="text-slate-500" />
        <span className="text-sm font-semibold">Sessions</span>
        <button
          type="button"
          aria-label="Create session"
          onClick={addSession}
          className="ml-auto rounded-lg p-1.5 text-slate-500 hover:bg-white"
        >
          <Plus size={15} />
        </button>
      </HeaderPortal>
      <div className="space-y-1 overflow-y-auto p-2" data-testid="session-list">
        {sessions.map((session) => (
          <button
            key={session.id}
            type="button"
            onClick={async () => {
              await selectSession(session.id);
              ensureInstance("chat", "slot-c");
            }}
            className={`flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left text-sm ${sessionId === session.id ? "bg-blue-50 text-blue-800" : "hover:bg-slate-100"}`}
          >
            <MessageSquare size={14} />
            <span className="min-w-0 flex-1 truncate">
              {session.title || "New session"}
            </span>
            <span className="shrink-0 text-[10px] text-slate-400">
              {agentTypeLabel(session.agent_type as Workspace["agent_type"])}
            </span>
          </button>
        ))}
        {sessions.length === 0 && (
          <p className="px-3 py-10 text-center text-xs leading-5 text-slate-500">
            No sessions yet.
            <br />
            Create one to begin.
          </p>
        )}
      </div>
    </div>
  );
}

function ChatApp() {
  const {
    workspace,
    sessions,
    sessionId,
    messages,
    loading,
    streaming,
    interrupting,
    error,
    setError,
    selectSession,
    addSession,
    send,
    interrupt,
    requests,
    resolveRequest,
  } = useRuntime();
  const activeSession = sessions.find((session) => session.id === sessionId);
  const compatible = isSessionCompatible(activeSession, workspace);
  const activeAgentType = activeSession?.agent_type ?? workspace.agent_type;
  const [input, setInput] = useState("");
  const bottom = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (messages.length) bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length]);
  async function submit(event: FormEvent) {
    event.preventDefault();
    const text = input.trim();
    if (!text || !sessionId || streaming) return;
    setInput("");
    await send(text);
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col px-4 pb-4">
      <HeaderPortal>
        <Sparkles size={15} className="text-slate-500" />
        <select
          aria-label="Active session"
          value={sessionId}
          onChange={(event) => selectSession(event.target.value)}
          className="min-w-0 max-w-48 bg-transparent text-sm font-semibold outline-none"
        >
          {sessions.length === 0 && <option value="">No session</option>}
          {sessions.map((session) => (
            <option key={session.id} value={session.id}>
              {session.title || "New session"} ·{" "}
              {agentTypeLabel(session.agent_type as Workspace["agent_type"])}
            </option>
          ))}
        </select>
        <button
          type="button"
          aria-label="New session"
          onClick={addSession}
          className="rounded-lg p-1.5 text-slate-500 hover:bg-white"
        >
          <Plus size={15} />
        </button>
      </HeaderPortal>
      <section
        data-testid="chat-messages"
        className="min-h-0 flex-1 space-y-4 overflow-y-auto py-5"
      >
        {loading && (
          <p className="text-center text-sm text-slate-500">
            Loading conversation…
          </p>
        )}
        {!loading && messages.length === 0 && (
          <div className="mx-auto mt-12 max-w-md text-center">
            <h2 className="text-2xl font-semibold tracking-tight">
              What shall we build?
            </h2>
            <p className="mt-3 text-sm leading-6 text-slate-500">
              {sessionId
                ? `Chat with ${agentTypeLabel(activeAgentType as Workspace["agent_type"])}.`
                : "Create or select a session to begin."}
            </p>
          </div>
        )}
        {messages.map((message) => (
          <MessageBubble key={message.id} message={message} />
        ))}
        <div ref={bottom} />
      </section>
      {error && (
        <div className="mb-3 flex justify-between rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
          <span>{error}</span>
          <button
            type="button"
            onClick={() => setError("")}
            aria-label="Dismiss error"
          >
            ×
          </button>
        </div>
      )}
      {requests
        .filter((request) => request.status === "pending")
        .map((request) => (
          <div key={request.request_id} className="mb-3">
            <AgentRequestCard
              request={request}
              onResolve={(response) => resolveRequest(request, response)}
            />
          </div>
        ))}
      {!streaming && sessionId && !compatible ? (
        <div
          role="status"
          className="flex flex-col gap-3 rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950 shadow-lg shadow-slate-200/40 sm:flex-row sm:items-center sm:justify-between"
        >
          <span>
            This session uses an older Agent configuration. Its history remains
            available, but new messages require a new session.
          </span>
          <Button type="button" onClick={() => void addSession()}>
            Start new session
          </Button>
        </div>
      ) : (
        <form
          onSubmit={submit}
          className="flex items-end gap-2 rounded-2xl border bg-white p-2 shadow-lg shadow-slate-200/60"
        >
          <textarea
            aria-label={`Message ${agentTypeLabel(activeAgentType as Workspace["agent_type"])}`}
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            placeholder={`Message ${agentTypeLabel(activeAgentType as Workspace["agent_type"])}…`}
            rows={1}
            className="max-h-40 min-h-11 flex-1 resize-none bg-transparent px-3 py-2.5 text-sm outline-none"
          />
          {streaming ? (
            <Button
              type="button"
              size="icon"
              variant="destructive"
              disabled={interrupting || !sessionId}
              aria-label={interrupting ? "Stopping agent" : "Stop agent"}
              onClick={() => void interrupt()}
            >
              <Square size={15} fill="currentColor" />
            </Button>
          ) : (
            <Button
              type="submit"
              size="icon"
              disabled={!input.trim() || !sessionId}
              aria-label="Send message"
            >
              <Send size={17} />
            </Button>
          )}
        </form>
      )}
    </div>
  );
}

function SettingsApp() {
  const { workspace, openSettings } = useRuntime();
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <HeaderPortal>
        <Settings size={15} className="text-slate-500" />
        <span className="text-sm font-semibold">Workspace settings</span>
      </HeaderPortal>
      <div
        data-testid="workspace-settings"
        className="mx-auto w-full max-w-xl space-y-5 overflow-y-auto p-6 sm:p-8"
      >
        <div>
          <h2 className="text-xl font-semibold">{workspace.name}</h2>
          <p className="mt-1 text-sm text-slate-500">
            Agent configuration for this workspace.
          </p>
        </div>
        <dl className="grid gap-4 rounded-2xl bg-slate-50 p-4 text-sm">
          <div>
            <dt className="text-xs uppercase tracking-wide text-slate-500">
              Agent
            </dt>
            <dd className="mt-1 font-medium capitalize">
              {workspace.agent_type}
            </dd>
          </div>
          <div>
            <dt className="text-xs uppercase tracking-wide text-slate-500">
              API key
            </dt>
            <dd className="mt-1 font-medium">
              {workspace.api_key_configured ? "Configured" : "Not configured"}
            </dd>
          </div>
        </dl>
        <Button onClick={openSettings}>Edit workspace</Button>
      </div>
    </div>
  );
}

const WORKSPACE_APPS = createAppRegistry([
  { id: "sessions", label: "Sessions", Component: SessionsApp },
  { id: "workspace-settings", label: "Settings", Component: SettingsApp },
  { id: "chat", label: "Chat", Component: ChatApp },
]);

function Menubar({
  workspaces,
  workspace,
  onSelect,
  onCreate,
  onEdit,
  onManageUsers,
}: {
  workspaces: Workspace[];
  workspace: Workspace | null;
  onSelect(id: string): void;
  onCreate(): void;
  onEdit(): void;
  onManageUsers?: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);
  const visible = workspaces.filter((item) =>
    item.name.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <header className="relative z-40 flex h-12 shrink-0 items-center justify-between border-b border-slate-200/80 bg-white/60 px-3 backdrop-blur-xl">
      <div className="flex items-center gap-2">
        <Bot size={18} />
        <span className="h-4 w-px bg-slate-200" />
        <div ref={root} className="relative">
          <button
            type="button"
            aria-label="Switch workspace"
            onClick={() => setOpen((value) => !value)}
            className="flex items-center gap-2 rounded-full border bg-white/70 px-3 py-1.5 text-sm font-medium hover:bg-white"
          >
            <span className="max-w-52 truncate">
              {workspace?.name ?? "All workspaces"}
            </span>
            <ChevronDown size={13} />
          </button>
          {open && (
            <div className="absolute left-0 top-full z-40 mt-2 w-80 overflow-hidden rounded-2xl border bg-white p-1 shadow-xl">
              <button
                type="button"
                onClick={() => {
                  onCreate();
                  setOpen(false);
                }}
                className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-sm hover:bg-slate-100"
              >
                <Plus size={15} />
                New workspace
              </button>
              {workspace && (
                <button
                  type="button"
                  onClick={() => {
                    onEdit();
                    setOpen(false);
                  }}
                  className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-sm hover:bg-slate-100"
                >
                  <Settings size={15} />
                  Workspace settings
                </button>
              )}
              {onManageUsers && (
                <button
                  type="button"
                  onClick={() => {
                    onManageUsers();
                    setOpen(false);
                  }}
                  className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-sm hover:bg-slate-100"
                >
                  <Users size={15} />
                  {t("users.heading")}
                </button>
              )}
              <div className="my-1 border-t" />
              <div className="flex items-center gap-2 px-3">
                <Search size={14} className="text-slate-400" />
                <input
                  aria-label="Search workspaces"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search workspaces…"
                  className="h-9 min-w-0 flex-1 text-sm outline-none"
                />
              </div>
              <div className="max-h-64 overflow-y-auto">
                {visible.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => {
                      onSelect(item.id);
                      setOpen(false);
                    }}
                    className={`flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm ${item.id === workspace?.id ? "bg-slate-900 text-white" : "hover:bg-slate-100"}`}
                  >
                    <span className="truncate">{item.name}</span>
                    <span
                      className={`text-xs ${item.id === workspace?.id ? "text-slate-300" : "text-slate-400"}`}
                    >
                      {item.agent_type}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
      <Button
        variant="ghost"
        size="sm"
        onClick={() =>
          api.logout().then(() => {
            window.location.href = "/login";
          })
        }
      >
        Sign out
      </Button>
    </header>
  );
}

function seedCompletedItems(
  history: ChatMessage[],
  target: Map<string, string>,
) {
  for (const message of history) {
    try {
      const content = JSON.parse(message.content_json) as { item_id?: string };
      if (content.item_id) target.set(content.item_id, message.id);
    } catch {
      // User messages and legacy plain text have no runtime item id.
    }
  }
}

export function ChatPage() {
  const { user } = useAuth();
  const params = useParams<{ workspaceId?: string }>();
  const workspaceId = params.workspaceId ?? "";
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedSessionId = searchParams.get("session");
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspacesLoaded, setWorkspacesLoaded] = useState(false);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [sessionId, setSessionId] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [historyReadySessionId, setHistoryReadySessionId] = useState("");
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [interrupting, setInterrupting] = useState(false);
  const [requests, setRequests] = useState<AgentRequest[]>([]);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<"create" | "edit" | null>(null);
  const requestVersion = useRef(0);
  const activeSessionId = useRef("");
  const itemMessageIds = useRef(new Map<string, string>());
  const itemTexts = useRef(new Map<string, string>());
  const eventCursors = useRef(new Map<string, number>());
  const workspace = workspaces.find((item) => item.id === workspaceId) ?? null;
  const workspaceExists = workspaces.some((item) => item.id === workspaceId);
  useEffect(() => {
    listWorkspaces()
      .then(setWorkspaces)
      .catch((cause: Error) => setError(cause.message))
      .finally(() => setWorkspacesLoaded(true));
  }, []);
  useEffect(() => {
    if (!workspaceId) {
      setSessions([]);
      activeSessionId.current = "";
      setSessionId("");
      setHistoryReadySessionId("");
      setMessages([]);
      setRequests([]);
      setStreaming(false);
      setInterrupting(false);
      return;
    }
    if (!workspacesLoaded) return;
    if (!workspaceExists) {
      navigate("/", { replace: true });
      return;
    }
    const version = ++requestVersion.current;
    setLoading(true);
    setMessages([]);
    setRequests([]);
    setHistoryReadySessionId("");
    setStreaming(false);
    setInterrupting(false);
    setError("");
    listSessions(workspaceId)
      .then(async (values) => {
        if (version !== requestVersion.current) return;
        setSessions(values);
        const selected =
          values.find((item) => item.id === requestedSessionId) ?? values[0];
        activeSessionId.current = selected?.id ?? "";
        setSessionId(selected?.id ?? "");
        setStreaming(selected?.chat_status === "agent");
        if (selected) {
          if (requestedSessionId !== selected.id)
            setSearchParams({ session: selected.id }, { replace: true });
          const history = await listMessages(selected.id);
          if (version === requestVersion.current) {
            seedCompletedItems(history, itemMessageIds.current);
            setMessages(history);
            setHistoryReadySessionId(selected.id);
            const pending = await listAgentRequests(selected.id);
            if (version === requestVersion.current) setRequests(pending);
          }
        } else {
          setRequests([]);
          setSearchParams({}, { replace: true });
        }
      })
      .catch((cause: Error) => {
        if (version === requestVersion.current) setError(cause.message);
      })
      .finally(() => {
        if (version === requestVersion.current) setLoading(false);
      });
  }, [
    workspaceId,
    workspaceExists,
    workspacesLoaded,
    navigate,
    requestedSessionId,
    setSearchParams,
  ]);

  const selectSession = useCallback(
    async (nextId: string) => {
      const version = ++requestVersion.current;
      activeSessionId.current = nextId;
      setSessionId(nextId);
      setStreaming(
        sessions.find((session) => session.id === nextId)?.chat_status ===
          "agent",
      );
      setInterrupting(false);
      setRequests([]);
      setSearchParams({ session: nextId }, { replace: true });
      const values = await listMessages(nextId);
      if (version === requestVersion.current) {
        itemMessageIds.current.clear();
        itemTexts.current.clear();
        seedCompletedItems(values, itemMessageIds.current);
        setMessages(values);
        setHistoryReadySessionId(nextId);
        const pending = await listAgentRequests(nextId);
        if (version === requestVersion.current) setRequests(pending);
      }
    },
    [sessions, setSearchParams],
  );
  async function addSession() {
    if (!workspaceId) return;
    setError("");
    try {
      const value = await createSession(
        workspaceId,
        `Session ${sessions.length + 1}`,
      );
      setSessions((current) => [
        value,
        ...current.filter((session) => session.id !== value.id),
      ]);
      await selectSession(value.id);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Session creation failed",
      );
    }
  }

  const applyEvent = useCallback(
    (targetSession: string, event: UniversalEvent) => {
      if (targetSession !== activeSessionId.current) return;
      if (event.type === "session.started") {
        setStreaming(true);
        setSessions((current) =>
          current.map((session) =>
            session.id === targetSession
              ? { ...session, chat_status: "agent" }
              : session,
          ),
        );
        void listMessages(targetSession).then((history) => {
          if (targetSession !== activeSessionId.current) return;
          seedCompletedItems(history, itemMessageIds.current);
          setMessages(history);
        });
      }
      if (
        event.type === "agent.requested" ||
        event.type === "agent.request.resolved"
      ) {
        void listAgentRequests(targetSession).then((values) => {
          if (targetSession === activeSessionId.current) setRequests(values);
        });
      }
      if (event.type === "error") {
        setError(event.message ?? "Agent failed");
        setStreaming(false);
        setInterrupting(false);
      }
      const delta = event.delta?.text;
      if (event.type === "item.delta" && delta) {
        const itemId = event.item_id ?? "assistant-current";
        const messageId =
          itemMessageIds.current.get(itemId) ?? createClientId();
        itemMessageIds.current.set(itemId, messageId);
        const nextText = (itemTexts.current.get(itemId) ?? "") + delta;
        itemTexts.current.set(itemId, nextText);
        const assistant: ChatMessage = {
          id: messageId,
          turn_id: "streaming",
          role: "assistant",
          content_json: JSON.stringify([{ type: "text", text: nextText }]),
          sequence: Date.now(),
          created_at: Date.now(),
        };
        setMessages((current) =>
          current.some((message) => message.id === messageId)
            ? current.map((message) =>
                message.id === messageId ? assistant : message,
              )
            : [...current, assistant],
        );
      }
      if (event.type === "item.completed" && event.item) {
        const text = itemText(event.item);
        if (text) {
          const itemId =
            event.item.item_id ?? event.item_id ?? createClientId();
          const messageId =
            itemMessageIds.current.get(itemId) ?? createClientId();
          itemMessageIds.current.set(itemId, messageId);
          const complete: ChatMessage = {
            id: messageId,
            turn_id: "streaming",
            role: String(event.item.role).toLowerCase().includes("tool")
              ? "tool"
              : "assistant",
            content_json: JSON.stringify(event.item),
            sequence: Date.now(),
            created_at: Date.now(),
          };
          setMessages((current) =>
            current.some((message) => message.id === messageId)
              ? current.map((message) =>
                  message.id === messageId ? complete : message,
                )
              : [...current, complete],
          );
        }
      }
      if (event.type === "session.ended") {
        setStreaming(false);
        setInterrupting(false);
        setSessions((current) =>
          current.map((session) =>
            session.id === targetSession
              ? { ...session, chat_status: "human" }
              : session,
          ),
        );
        void listMessages(targetSession).then((history) => {
          if (targetSession !== activeSessionId.current) return;
          seedCompletedItems(history, itemMessageIds.current);
          setMessages(history);
        });
      }
    },
    [],
  );

  useEffect(() => {
    if (!sessionId || historyReadySessionId !== sessionId) return;
    const controller = new AbortController();
    let stopped = false;
    const subscribe = async () => {
      while (!stopped) {
        const after = eventCursors.current.get(sessionId) ?? 0;
        try {
          const cursor = await subscribeSessionEvents(
            sessionId,
            after,
            controller.signal,
            (event, sequence) => {
              eventCursors.current.set(sessionId, sequence);
              applyEvent(sessionId, event);
            },
          );
          eventCursors.current.set(sessionId, cursor);
        } catch (cause) {
          if (controller.signal.aborted) return;
          setError(
            cause instanceof Error
              ? cause.message
              : "Event subscription failed",
          );
        }
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
    };
    void subscribe();
    return () => {
      stopped = true;
      controller.abort();
    };
  }, [applyEvent, historyReadySessionId, sessionId]);

  async function send(text: string) {
    const targetSession = activeSessionId.current;
    if (!targetSession || streaming) return;
    setError("");
    setStreaming(true);
    setInterrupting(false);
    setSessions((current) =>
      current.map((session) =>
        session.id === targetSession
          ? { ...session, chat_status: "agent" }
          : session,
      ),
    );
    const user: ChatMessage = {
      id: createClientId(),
      turn_id: "pending",
      role: "user",
      content_json: JSON.stringify([{ type: "text", text }]),
      sequence: Date.now(),
      created_at: Date.now(),
    };
    itemMessageIds.current.clear();
    itemTexts.current.clear();
    setMessages((current) => [...current, user]);
    try {
      await runTurn(targetSession, text);
    } catch (cause) {
      if (targetSession === activeSessionId.current) {
        setError(cause instanceof Error ? cause.message : "Chat failed");
        setStreaming(false);
        setSessions((current) =>
          current.map((session) =>
            session.id === targetSession
              ? { ...session, chat_status: "human" }
              : session,
          ),
        );
      }
    }
  }
  async function interrupt() {
    const targetSession = activeSessionId.current;
    if (!targetSession || !streaming || interrupting) return;
    setError("");
    setInterrupting(true);
    try {
      await interruptTurn(targetSession);
    } catch (cause) {
      if (targetSession === activeSessionId.current) {
        setInterrupting(false);
        setError(cause instanceof Error ? cause.message : "Stop failed");
      }
    }
  }
  async function resolveRequest(
    request: AgentRequest,
    response: Record<string, unknown>,
  ) {
    const targetSession = activeSessionId.current;
    const resolved = await resolveAgentRequest(
      targetSession,
      request.turn_id,
      request.request_id,
      response,
    );
    if (targetSession === activeSessionId.current) {
      setRequests((current) =>
        current.map((item) =>
          item.request_id === resolved.request_id ? resolved : item,
        ),
      );
    }
  }
  async function saved(value: Workspace) {
    const previous = workspaces.find((item) => item.id === value.id);
    setWorkspaces((current) =>
      current.some((item) => item.id === value.id)
        ? current.map((item) => (item.id === value.id ? value : item))
        : [value, ...current],
    );
    if (!workspaceId) {
      navigate(`/w/${value.id}`);
      return;
    }
    if (
      previous &&
      previous.configuration_version !== value.configuration_version
    ) {
      setError("");
      try {
        const created = await createSession(
          value.id,
          `Session ${sessions.length + 1}`,
        );
        setSessions((current) => [
          created,
          ...current.filter((session) => session.id !== created.id),
        ]);
        await selectSession(created.id);
      } catch {
        try {
          const current = await listSessions(value.id);
          setSessions(current);
          const compatibleSession = current.find((session) =>
            isSessionCompatible(session, value),
          );
          if (compatibleSession) {
            await selectSession(compatibleSession.id);
            return;
          }
        } catch {
          // The actionable error below covers both recovery failures.
        }
        setError(
          "Agent configuration was saved, but a new session could not be started. Start a new session to continue.",
        );
      }
    }
  }
  const runtime = workspace
    ? {
        workspace,
        sessions,
        sessionId,
        messages,
        loading,
        streaming,
        interrupting,
        error,
        setError,
        selectSession,
        addSession,
        send,
        interrupt,
        requests,
        resolveRequest,
        openSettings: () => setDialog("edit"),
      }
    : null;
  return (
    <main
      data-testid="chat-page"
      className="flex h-screen flex-col overflow-hidden bg-[radial-gradient(circle_at_top_left,_#dbeafe_0,_#f8fafc_42%,_#f8fafc_100%)] text-slate-950"
    >
      <Menubar
        workspaces={workspaces}
        workspace={workspace}
        onSelect={(id) => navigate(`/w/${id}`)}
        onCreate={() => setDialog("create")}
        onEdit={() => setDialog("edit")}
        onManageUsers={
          user?.role === "admin" ? () => navigate("/admin/users") : undefined
        }
      />
      {workspace && runtime ? (
        <RuntimeContext.Provider value={runtime}>
          <SlotProvider
            key={workspace.id}
            workspaceId={workspace.id}
            apps={WORKSPACE_APPS}
            configs={SLOT_CONFIGS}
          >
            <WorkspaceLayout />
            <SegmentedDock />
          </SlotProvider>
        </RuntimeContext.Provider>
      ) : (
        <div className="grid min-h-0 flex-1 place-items-center p-6">
          <div className="max-w-md text-center">
            <div className="mx-auto mb-4 grid h-12 w-12 place-items-center rounded-2xl bg-slate-900 text-white">
              <Bot size={22} />
            </div>
            <h1 className="text-2xl font-semibold">Choose a workspace</h1>
            <p className="mt-2 text-sm leading-6 text-slate-500">
              Select a workspace from the menu or create one to start working
              with an agent.
            </p>
            <Button className="mt-5" onClick={() => setDialog("create")}>
              <Plus size={15} />
              New workspace
            </Button>
          </div>
        </div>
      )}
      <WorkspaceFormDialog
        mode={dialog === "edit" ? "edit" : "create"}
        workspace={workspace ?? undefined}
        open={dialog !== null}
        onClose={() => setDialog(null)}
        onSaved={saved}
      />
    </main>
  );
}
