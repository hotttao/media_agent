import { describe, expect, it } from "vitest";
import type { ChatSession } from "@/domains/chat/types";
import { isSessionCompatible } from "./session-compatibility";

const workspace = {
  agent_type: "claude-code",
  configuration_version: 2,
};

const session: ChatSession = {
  id: "session",
  workspace_id: "workspace",
  agent_id: "claude-code",
  agent_type: "claude-code",
  title: "Session",
  status: "active",
  chat_status: "human",
  context_version: 2,
  created_at: 1,
  updated_at: 1,
};

describe("isSessionCompatible", () => {
  it("keeps pinned metadata sessions usable and requires a new session for a changed runtime type", () => {
    expect(isSessionCompatible(session, workspace)).toBe(true);
    expect(
      isSessionCompatible({ ...session, context_version: 1 }, workspace),
    ).toBe(true);
    expect(
      isSessionCompatible({ ...session, agent_type: "codex" }, workspace),
    ).toBe(false);
  });
});
