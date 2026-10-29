import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  createToken,
  listTokens,
  revokeToken,
} from "@/domains/api-token/lib/client";
import { TokenManager } from "./token-manager";

vi.mock("react-i18next", () => {
  const t = (key: string) => key;
  return { useTranslation: () => ({ t }) };
});
vi.mock("@/domains/api-token/lib/client", () => ({
  createToken: vi.fn(),
  listTokens: vi.fn(),
  revokeToken: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listTokens).mockResolvedValue([]);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
async function submit() {
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  fireEvent.change(screen.getByLabelText("apiTokens.name"), {
    target: { value: "My Agent" },
  });
  fireEvent.change(screen.getByLabelText("apiTokens.expires"), {
    target: { value: "2099-01-01T10:00" },
  });
  fireEvent.click(screen.getByRole("button", { name: "apiTokens.create" }));
}
it("issues only the explicit scope and forgets the secret on close", async () => {
  vi.mocked(createToken).mockResolvedValue({
    api_token: {
      id: "t",
      name: "My Agent",
      scopes: ["mcp.access"],
      status: "active",
      expires_at: "2099-01-01",
    },
    secret: "secret-once",
  });
  render(<TokenManager />);
  await submit();
  await screen.findByTestId("token-secret");
  expect(createToken).toHaveBeenCalledTimes(1);
  expect(createToken).toHaveBeenCalledWith(
    expect.objectContaining({
      scopes: ["mcp.access"],
      idempotency_key: expect.any(String),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "apiTokens.close" }));
  expect(screen.queryByText("secret-once")).toBeNull();
  expect(localStorage.getItem("secret-once")).toBeNull();
});
it("does not retry an uncertain issuance and reloads the server list", async () => {
  vi.mocked(createToken).mockRejectedValue(new Error("lost response"));
  render(<TokenManager />);
  await submit();
  await screen.findByText("apiTokens.unknown");
  fireEvent.click(screen.getByRole("button", { name: "apiTokens.create" }));
  expect(createToken).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(listTokens).toHaveBeenCalledTimes(2));
});
it("revokes only after confirmation and refreshes metadata", async () => {
  vi.mocked(listTokens).mockResolvedValue([
    {
      id: "t",
      name: "Agent",
      scopes: ["mcp.access"],
      status: "active",
      expires_at: "2099-01-01",
    },
  ]);
  vi.spyOn(window, "confirm").mockReturnValue(true);
  vi.mocked(revokeToken).mockResolvedValue({ success: true });
  render(<TokenManager />);
  fireEvent.click(
    await screen.findByRole("button", { name: "apiTokens.revoke" }),
  );
  await waitFor(() => expect(revokeToken).toHaveBeenCalledWith("t"));
  await waitFor(() => expect(listTokens).toHaveBeenCalledTimes(2));
});

it("supports HTTP origins without randomUUID and reports unavailable clipboard", async () => {
  const getRandomValues = globalThis.crypto.getRandomValues.bind(
    globalThis.crypto,
  );
  vi.stubGlobal("crypto", { getRandomValues });
  vi.mocked(createToken).mockResolvedValue({
    api_token: {
      id: "t",
      name: "Agent",
      scopes: ["mcp.access"],
      status: "active",
      expires_at: "2099-01-01",
    },
    secret: "secret-once",
  });
  render(<TokenManager />);
  await submit();
  await screen.findByTestId("token-secret");
  expect(createToken).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "apiTokens.copy" }));
  await screen.findByText("apiTokens.copyFailed");
});
