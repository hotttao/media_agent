import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workspace } from "@/domains/workspace/types";
import { WorkspaceFormDialog } from "./workspace-form-dialog";

const claudeWorkspace: Workspace = {
  id: "workspace-1",
  name: "Claude workspace",
  agent_id: "claude-code",
  agent_type: "claude-code",
  api_key_configured: true,
  configuration_version: 2,
  created_at: 1,
  updated_at: 2,
};

afterEach(() => vi.restoreAllMocks());

describe("WorkspaceFormDialog", () => {
  it("creates a Claude Code workspace without persisting or echoing the API key", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(claudeWorkspace), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const onSaved = vi.fn();
    render(
      <WorkspaceFormDialog
        mode="create"
        open
        onClose={vi.fn()}
        onSaved={onSaved}
      />,
    );

    expect((screen.getByLabelText("Agent") as HTMLSelectElement).value).toBe(
      "codex",
    );
    fireEvent.change(screen.getByLabelText("Workspace name"), {
      target: { value: "Claude workspace" },
    });
    fireEvent.change(screen.getByLabelText("Agent"), {
      target: { value: "claude-code" },
    });
    fireEvent.change(screen.getByLabelText("API key"), {
      target: { value: "sk-ant-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(claudeWorkspace));
    const request = fetchMock.mock.calls[0]?.[1];
    expect(JSON.parse(String(request?.body))).toEqual({
      name: "Claude workspace",
      agent_type: "claude-code",
      api_key: "sk-ant-secret",
    });
    expect(onSaved.mock.calls[0]?.[0]).not.toHaveProperty("api_key");
  });

  it("loads the current Agent Type and clears the secret when reopened", () => {
    const properties = {
      mode: "edit" as const,
      workspace: claudeWorkspace,
      onClose: vi.fn(),
      onSaved: vi.fn(),
    };
    const { rerender } = render(<WorkspaceFormDialog {...properties} open />);

    expect((screen.getByLabelText("Agent") as HTMLSelectElement).value).toBe(
      "claude-code",
    );
    expect(
      (screen.getByLabelText("Settings API key") as HTMLInputElement).value,
    ).toBe("");
    expect(
      screen.getByPlaceholderText("Configured — leave blank to keep"),
    ).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Settings API key"), {
      target: { value: "temporary-secret" },
    });

    rerender(<WorkspaceFormDialog {...properties} open={false} />);
    rerender(<WorkspaceFormDialog {...properties} open />);
    expect(
      (screen.getByLabelText("Settings API key") as HTMLInputElement).value,
    ).toBe("");
  });

  it("keeps form values in memory when saving fails", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "configuration rejected" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      }),
    );
    render(
      <WorkspaceFormDialog
        mode="create"
        open
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Workspace name"), {
      target: { value: "Keep me" },
    });
    fireEvent.change(screen.getByLabelText("Agent"), {
      target: { value: "claude-code" },
    });
    fireEvent.change(screen.getByLabelText("API key"), {
      target: { value: "temporary-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(await screen.findByText("configuration rejected")).toBeTruthy();
    expect((screen.getByLabelText("Agent") as HTMLSelectElement).value).toBe(
      "claude-code",
    );
    expect((screen.getByLabelText("API key") as HTMLInputElement).value).toBe(
      "temporary-secret",
    );
  });
});
