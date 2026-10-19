import { storedContentText } from "@/domains/chat/lib/chat-client";
import type { ChatMessage } from "@/domains/chat/types";

export function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user";
  return (
    <article className={`flex min-w-0 ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`min-w-0 max-w-[82%] whitespace-pre-wrap rounded-2xl px-4 py-3 text-sm leading-6 shadow-sm [overflow-wrap:anywhere] ${
          isUser
            ? "bg-slate-900 text-white"
            : message.role === "tool"
              ? "border bg-amber-50 text-amber-950"
              : "border bg-white"
        }`}
      >
        <div className="mb-1 text-[10px] font-semibold uppercase tracking-[0.16em] opacity-55">
          {message.role}
        </div>
        {storedContentText(message.content_json)}
      </div>
    </article>
  );
}
