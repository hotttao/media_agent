import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Product } from "@/domains/products/types";
import { ProductWorkflowLaunch } from "./workflow-launch";
const fixture = vi.hoisted(() => ({
  openWorkflowRun: vi.fn(),
  owner: "owner",
  navigation: true,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useCurrentUser: () => ({ user: { id: fixture.owner } }),
    useNavigationPort: () =>
      fixture.navigation
        ? { openWorkflowRun: fixture.openWorkflowRun }
        : undefined,
  },
}));
const workflow = {
  id: "wf",
  name: "Fashion video",
  scope: "media",
  version: 1,
  status: "published",
  publication_status: "registered",
  organization_id: "",
  created_by: "owner",
  my_permission: "owner",
  allowed_actions: ["run"],
  input_schema: { entries: [{ name: "product", resource_type: "PRODUCT" }] },
};
vi.mock("@/domains/workflows/lib/api", () => ({
  listWorkflows: () =>
    Promise.resolve({
      items: [
        { ...workflow, my_permission: undefined, allowed_actions: undefined },
      ],
      total: 1,
    }),
}));
vi.mock("@/domains/workflows/hooks/use-workflow", () => ({
  useWorkflow: () => ({ data: workflow }),
}));
afterEach(() => {
  cleanup();
  fixture.openWorkflowRun.mockReset();
  fixture.owner = "owner";
  fixture.navigation = true;
});
function open() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ProductWorkflowLaunch
        product={
          {
            id: "product",
            name: "Dress",
            scope: "media",
            status: "active",
          } as Product
        }
        organizationId="org"
      />
    </QueryClientProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "productWorkflow.choose" }),
  );
}
describe("product compatible workflow cards", () => {
  it("searches authorized owner legacy definitions and opens a prefilled intent without submitting", async () => {
    open();
    const card = await screen.findByRole("button", { name: /Fashion video/ });
    fireEvent.change(
      screen.getByRole("textbox", { name: "productWorkflow.search" }),
      { target: { value: "missing" } },
    );
    expect(screen.queryByRole("button", { name: /Fashion video/ })).toBeNull();
    fireEvent.change(
      screen.getByRole("textbox", { name: "productWorkflow.search" }),
      { target: { value: "Fashion" } },
    );
    fireEvent.click(screen.getByRole("button", { name: /Fashion video/ }));
    expect(card).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "productWorkflow.openRun" }),
    );
    expect(fixture.openWorkflowRun).toHaveBeenCalledExactlyOnceWith({
      organizationId: "org",
      workflowId: "wf",
      productId: "product",
    });
  });
  it("does not expose an unscoped legacy definition belonging to another user", async () => {
    fixture.owner = "other";
    open();
    await waitFor(() =>
      expect(screen.getByText("productWorkflow.empty")).toBeTruthy(),
    );
    expect(screen.queryByRole("button", { name: /Fashion video/ })).toBeNull();
  });
});
