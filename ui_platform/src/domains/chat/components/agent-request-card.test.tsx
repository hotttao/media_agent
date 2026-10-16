import "@/foundation/lib/i18n";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRequest } from "@/domains/chat/types";
import { AgentRequestCard } from "./agent-request-card";

const request: AgentRequest = {
  request_id: "request-1",
  session_id: "session-1",
  turn_id: "turn-1",
  request_type: "permission",
  payload: {
    question: "Allow the agent to run this command?",
    options: [{ option_id: "allow", label: "Allow once" }],
  },
  status: "pending",
  response: null,
  resolved_at: null,
  created_at: "2026-08-22T00:00:00Z",
};

describe("AgentRequestCard", () => {
  it("shows native ACP tool title before permission is granted", () => {
    render(
      <AgentRequestCard
        request={{
          ...request,
          payload: {
            tool_call: {
              title: "Read project instructions",
              raw_input: { private: "must-not-render" },
            },
            options: [{ option_id: "allow", name: "Allow once" }],
          },
        }}
        onResolve={vi.fn()}
      />,
    );
    expect(screen.getByText("Read project instructions")).toBeTruthy();
    expect(screen.queryByText("must-not-render")).toBeNull();
  });
  it("resolves a permission option without persisting UI state", async () => {
    const onResolve = vi.fn().mockResolvedValue(undefined);
    render(<AgentRequestCard request={request} onResolve={onResolve} />);

    fireEvent.click(screen.getByRole("button", { name: "Allow once" }));

    await waitFor(() =>
      expect(onResolve).toHaveBeenCalledWith({ option_id: "allow" }),
    );
  });

  it("shows the requested command when the native title is absent", () => {
    render(
      <AgentRequestCard
        request={{
          ...request,
          payload: {
            tool_call: {
              title: null,
              raw_input: {
                command: ["printf", "approved"],
                private: "must-not-render",
              },
            },
            options: [],
          },
        }}
        onResolve={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Command to approve").textContent).toContain(
      "approved",
    );
    expect(screen.queryByText("must-not-render")).toBeNull();
  });

  it("submits input responses", async () => {
    const onResolve = vi.fn().mockResolvedValue(undefined);
    render(
      <AgentRequestCard
        request={{
          ...request,
          request_type: "input",
          payload: { prompt: "Value?" },
        }}
        onResolve={onResolve}
      />,
    );

    fireEvent.change(screen.getByLabelText("Agent input"), {
      target: { value: "answer" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(onResolve).toHaveBeenCalledWith({ value: "answer" }),
    );
  });
});
