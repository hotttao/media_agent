import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { listMaterials } from "@/domains/materials/lib/api";
import type { WorkflowOption } from "@/domains/jobs/types";
import { useJobInputOptions } from "./use-job-input-options";

vi.mock("@/domains/materials/lib/api", () => ({ listMaterials: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("isolates candidates by company and projected frozen input contract", async () => {
  vi.mocked(listMaterials).mockImplementation(
    async (params = {}) =>
      ({
        items: [
          {
            id: params.asset_kind,
            name: params.asset_kind,
            scope: "test",
            asset_kind: params.asset_kind,
          },
        ],
        total: 1,
      }) as Awaited<ReturnType<typeof listMaterials>>,
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result, rerender } = renderHook(
    ({ kind, organization }) => {
      const workflow: WorkflowOption = {
        id: "same-workflow",
        name: "Frozen",
        scope: "test",
        version: 1,
        status: 2,
        input_schema: {
          entries: [
            {
              name: kind,
              resource_type: "MATERIAL",
              asset_kind: kind,
              required: true,
            },
          ],
        },
      };
      return useJobInputOptions(workflow, true, organization);
    },
    { wrapper, initialProps: { kind: "movement", organization: "one" } },
  );
  await waitFor(() =>
    expect(result.current.data?.movement[0].id).toBe("movement"),
  );
  rerender({ kind: "pose", organization: "one" });
  await waitFor(() => expect(result.current.data?.pose[0].id).toBe("pose"));
  expect(result.current.data?.movement).toBeUndefined();
  rerender({ kind: "pose", organization: "two" });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(listMaterials).toHaveBeenCalledTimes(3);
});
