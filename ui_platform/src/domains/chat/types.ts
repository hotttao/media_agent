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
}

export interface ChatMessage {
  id: string;
  turn_id: string;
  role: "user" | "assistant" | "tool";
  content_json: string;
  sequence: number;
  created_at: number;
}

export interface UniversalEvent {
  type: string;
  item_id?: string;
  message?: string;
  item?: {
    item_id?: string;
    role?: string | number;
    kind?: string | number;
    content?: Array<{ type?: string | number; text?: string; output?: string }>;
  };
  delta?: { text?: string };
}
