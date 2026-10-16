export interface ChatSession {
  id: string;
  workspace_id: string;
  agent_id: string;
  status: string;
  created_at: number;
  updated_at: number;
  title: string;
  agent_type: string;
  chat_status: "human" | "agent";
  context_version: number;
  runtime_status?: string;
  execution_epoch?: number;
}

export interface ChatMessage {
  id: string;
  turn_id: string;
  role: "user" | "assistant" | "tool";
  content_json: string;
  sequence: number;
  created_at: number;
}

export type AgentRequestType = "confirm" | "input" | "permission" | string;

export interface AgentRequest {
  request_id: string;
  session_id: string;
  turn_id: string;
  request_type: AgentRequestType;
  payload: Record<string, unknown>;
  status: "pending" | "resolved" | "cancelled" | "expired" | string;
  response: Record<string, unknown> | null;
  resolved_at: string | null;
  created_at: string;
}

export interface UniversalEvent {
  type: string;
  item_id?: string;
  message?: string;
  error_class?: string;
  item?: {
    item_id?: string;
    role?: string | number;
    kind?: string | number;
    content?: Array<{ type?: string | number; text?: string; output?: string }>;
  };
  delta?: { text?: string };
}
