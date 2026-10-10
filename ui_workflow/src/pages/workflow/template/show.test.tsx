import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import type { Template } from "@/domains/templates/types";
import { TemplateShow } from "./show";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useShellContext: () => ({ organization: { id: "org" } }),
    useAppHeaderSlot: () => null,
  },
}));
vi.mock("@/domains/templates/hooks/use-template", () => ({
  useTemplate: vi.fn(),
}));
vi.mock("@/domains/templates/hooks/use-template-mutations", () => ({
  useArchiveTemplate: () => ({ isPending: false }),
  useChangeTemplateVisibility: () => ({ isPending: false }),
  usePublishTemplate: () => ({ isPending: false }),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
describe("template actions use service projections", () => {
  it.each([
    ["viewer", ["read"], false, false],
    ["editor", ["read", "update", "archive"], true, false],
    ["owner", ["read", "update", "publish", "archive"], true, true],
    ["owner", ["read", "archive", "revoke"], false, false],
    [undefined, undefined, false, false],
  ])("does not infer actions for %s", (permission, actions, edit, publish) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.mocked(useTemplate).mockReturnValue({
      data: {
        id: "tpl",
        created_by: "current-user",
        version: 1,
        status: "draft",
        name: "Test",
        scope: "scope",
        visibility: "private",
        inputs: [],
        invocation_mapping: {},
        my_permission: permission,
        allowed_actions: actions,
      } as unknown as Template,
      isLoading: false,
      isError: false,
    } as ReturnType<typeof useTemplate>);
    render(
      <QueryClientProvider client={client}>
        <TemplateShow id="tpl" onBack={() => {}} onEdit={() => {}} />
      </QueryClientProvider>,
    );
    expect(
      Boolean(screen.queryByRole("button", { name: "templateView.edit" })),
    ).toBe(edit);
    expect(
      Boolean(screen.queryByRole("button", { name: "templateView.publish" })),
    ).toBe(publish);
    client.clear();
  });
});
