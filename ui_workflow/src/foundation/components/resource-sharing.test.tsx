import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ResourceSharing } from "./resource-sharing";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, vars?: { id?: string }) =>
      vars?.id ? `${key} ${vars.id}` : key,
  }),
}));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount(
  owner = true,
  actions = ["grant", "revoke", "change_visibility"],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response(JSON.stringify({ teams: [] }))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  let rows = [{ team_id: "missing-dept", permission: "editor" as const }];
  const load = vi.fn(async () => rows);
  const revoke = vi.fn(async () => {
    rows = [];
  });
  const grant = vi.fn(async () => {});
  render(
    <QueryClientProvider client={client}>
      <ResourceSharing
        organizationId="org"
        visibility="private"
        owner={owner}
        allowedActions={actions}
        resourceKey={["grants", "wf"]}
        loadGrants={load}
        onGrant={grant}
        onRevoke={revoke}
        onVisibility={async () => {}}
      />
    </QueryClientProvider>,
  );
  return { client, load, revoke, grant };
}
describe("persisted resource sharing", () => {
  it("can revoke a persisted grant even when the department directory is empty", async () => {
    const { client, load, revoke } = mount();
    await screen.findByText("sharing.unknownDepartment missing-dept");
    expect(screen.getByText("sharing.privateGrants")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "sharing.revoke" }));
    await screen.findByText("sharing.noGrants");
    expect(revoke).toHaveBeenCalledWith("missing-dept");
    expect(load).toHaveBeenCalledTimes(2);
    client.clear();
  });
  it("never queries owner grants for a readable non-owner", async () => {
    const { client, load } = mount(false);
    expect(screen.getByText("sharing.ownerOnly")).toBeTruthy();
    await waitFor(() => expect(load).not.toHaveBeenCalled());
    expect(screen.queryByText("sharing.grants")).toBeNull();
    client.clear();
  });
  it("does not infer allowed commands from owner identity", async () => {
    const { client, revoke } = mount(true, []);
    await screen.findByText("sharing.unknownDepartment missing-dept");
    const button = screen.getByRole("button", { name: "sharing.revoke" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(button);
    expect(revoke).not.toHaveBeenCalled();
    client.clear();
  });
});
