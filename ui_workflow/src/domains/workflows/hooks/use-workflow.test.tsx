import { renderHook, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getWorkflow, getWorkflowVersion } from "../lib/api";
import type { Workflow } from "../types";
import { useWorkflow } from "./use-workflow";

vi.mock("../lib/api", () => ({
  getWorkflow: vi.fn(),
  getWorkflowVersion: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Workflow version query ownership", () => {
  it("separates current and frozen cache keys and never substitutes head data", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    vi.mocked(getWorkflow).mockResolvedValue({
      id: "wf",
      version: 3,
      name: "current",
    } as Workflow);
    vi.mocked(getWorkflowVersion).mockImplementation(
      async (_id, version) =>
        ({ id: "wf", version, name: `frozen-${version}` }) as Workflow,
    );
    const query = renderHook(
      ({ version }: { version: number | undefined }) =>
        useWorkflow("wf", version),
      { wrapper, initialProps: { version: undefined as number | undefined } },
    );
    await waitFor(() =>
      expect(query.result.current.data?.name).toBe("current"),
    );
    query.rerender({ version: 1 });
    expect(query.result.current.data).toBeUndefined();
    await waitFor(() =>
      expect(query.result.current.data?.name).toBe("frozen-1"),
    );
    query.rerender({ version: 2 });
    expect(query.result.current.data).toBeUndefined();
    await waitFor(() =>
      expect(query.result.current.data?.name).toBe("frozen-2"),
    );
    expect(getWorkflow).toHaveBeenCalledTimes(1);
    expect(getWorkflowVersion).toHaveBeenCalledWith("wf", 1);
    expect(getWorkflowVersion).toHaveBeenCalledWith("wf", 2);
    client.clear();
  });
  it("keeps a missing version failed even when the newer head is cached", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["workflow", "wf"], { id: "wf", version: 2 });
    vi.mocked(getWorkflowVersion).mockRejectedValue(
      new Error("version missing"),
    );
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const query = renderHook(() => useWorkflow("wf", 1), { wrapper });
    await waitFor(() => expect(query.result.current.isError).toBe(true));
    expect(query.result.current.data).toBeUndefined();
    expect(getWorkflow).not.toHaveBeenCalled();
    client.clear();
  });
});
