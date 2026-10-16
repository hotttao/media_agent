import "@/foundation/lib/i18n";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input) => {
        const path = String(input);
        const body =
          path === "/api/organizations"
            ? {
                organizations: [
                  {
                    id: "org",
                    name: "Company",
                    status: "active",
                    my_role: "member",
                  },
                ],
              }
            : path === "/api/teams"
              ? {
                  teams: [
                    {
                      id: "team",
                      name: "Creative",
                      organization_id: "org",
                      status: "active",
                      my_role: "member",
                    },
                  ],
                }
              : claudeWorkspace;
        return new Response(JSON.stringify(body), {
          status: path === "/api/v1/workspaces" ? 201 : 200,
          headers: { "Content-Type": "application/json" },
        });
      });
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
    await waitFor(() =>
      expect(
        (
          screen.getByRole("button", {
            name: "Create workspace",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false),
    );
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(claudeWorkspace));
    const request = fetchMock.mock.calls.find(
      (call) => String(call[0]) === "/api/v1/workspaces",
    )?.[1];
    expect(JSON.parse(String(request?.body))).toEqual({
      name: "Claude workspace",
      agent_type: "claude-code",
      api_key: "sk-ant-secret",
      model_connection_id: "",
      organization_id: "org",
      team_id: "team",
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
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = String(input);
      const body =
        path === "/api/organizations"
          ? {
              organizations: [
                {
                  id: "org",
                  name: "Company",
                  status: "active",
                  my_role: "member",
                },
              ],
            }
          : path === "/api/teams"
            ? {
                teams: [
                  {
                    id: "team",
                    name: "Creative",
                    organization_id: "org",
                    status: "active",
                    my_role: "member",
                  },
                ],
              }
            : { error: "configuration rejected" };
      return new Response(JSON.stringify(body), {
        status: path === "/api/v1/workspaces" ? 400 : 200,
        headers: { "Content-Type": "application/json" },
      });
    });
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
    await waitFor(() =>
      expect(
        (
          screen.getByRole("button", {
            name: "Create workspace",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false),
    );
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

it("requires explicit personal connection selection and clears the API key alternative", async () => {
  const workspace = {
    ...claudeWorkspace,
    name: "Codex workspace",
    agent_id: "codex",
    agent_type: "codex" as const,
  };
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(
    async (input) =>
      new Response(
        JSON.stringify(
          String(input) === "/api/v1/model-connections"
            ? {
                connections: [
                  {
                    id: "connection",
                    name: "本机 Codex",
                    kind: "codex_auth_json",
                    status: "active",
                    version: 1,
                  },
                ],
              }
            : {
                ...workspace,
                model_connection_id: "connection",
                api_key_configured: false,
              },
        ),
        { status: 200 },
      ),
  );
  const onSaved = vi.fn();
  render(
    <MemoryRouter>
      <WorkspaceFormDialog
        mode="edit"
        workspace={workspace}
        open
        onClose={vi.fn()}
        onSaved={onSaved}
      />
    </MemoryRouter>,
  );
  fireEvent.change(screen.getByLabelText("Settings API key"), {
    target: { value: "discarded-key" },
  });
  fireEvent.change(screen.getByLabelText("Model authentication"), {
    target: { value: "connection" },
  });
  await screen.findByRole("option", { name: "本机 Codex" });
  expect(
    (screen.getByLabelText("Model connection") as HTMLSelectElement).value,
  ).toBe("");
  expect(
    (screen.getByRole("button", { name: "Save changes" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.change(screen.getByLabelText("Model connection"), {
    target: { value: "connection" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalled());
  const call = fetchMock.mock.calls.find((c) => c[1]?.method === "PUT");
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({
    name: "Codex workspace",
    agent_type: "codex",
    api_key: "",
    model_connection_id: "connection",
  });
});
