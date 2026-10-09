import { renderHook, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { standaloneShim } from "./standalone";
import { pluginShim } from "./plugin";
import { WORKFLOW_LAUNCH_KEY } from "@/foundation/types/app-navigation";
import { useSlotContext } from "@/foundation/providers/slot-provider";
import { setInstanceState } from "@/foundation/stores/instance-state-store";

vi.mock("@/foundation/providers/slot-provider", () => ({
  useSlotContext: vi.fn(),
}));
vi.mock("@/foundation/stores/instance-state-store", () => ({
  setInstanceState: vi.fn(),
  useInstanceState: vi.fn(),
  useInstancePersistentState: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("app navigation adapters", () => {
  it("seeds only the target instance before activating it, without submitting a job", () => {
    const ensureInstance = vi
      .fn()
      .mockReturnValue({ slotId: "slot-b", instanceId: "workflow-instance" });
    const activate = vi.fn();
    vi.mocked(useSlotContext).mockReturnValue({
      workspaceId: "profile-workspace",
      ensureInstance,
      activate,
    } as unknown as ReturnType<typeof useSlotContext>);
    const { result } = renderHook(() => standaloneShim.useNavigationPort());
    result.current!.openWorkflowRun({
      organizationId: "org",
      workflowId: "workflow",
      productId: "product",
    });
    expect(ensureInstance).toHaveBeenCalledWith("workflow");
    expect(setInstanceState).toHaveBeenCalledWith(
      "profile-workspace",
      "workflow-instance",
      WORKFLOW_LAUNCH_KEY,
      {
        organizationId: "org",
        workflowId: "workflow",
        productId: "product",
        kind: "workflow_run",
        requestId: expect.any(String),
      },
    );
    expect(activate).toHaveBeenCalledWith("slot-b", "workflow-instance");
    expect(
      vi.mocked(setInstanceState).mock.invocationCallOrder[0],
    ).toBeLessThan(activate.mock.invocationCallOrder[0]);
  });
  it("does not invent navigation when an older plugin host omits the optional port", () => {
    vi.stubGlobal("window", { tos: {} });
    expect(pluginShim.useNavigationPort()).toBeUndefined();
    const openWorkflowRun = vi.fn();
    vi.stubGlobal("window", { tos: { navigation: { openWorkflowRun } } });
    pluginShim
      .useNavigationPort()!
      .openWorkflowRun({
        organizationId: "org",
        workflowId: "workflow",
        productId: "product",
      });
    expect(openWorkflowRun).toHaveBeenCalledWith({
      organizationId: "org",
      workflowId: "workflow",
      productId: "product",
    });
  });
});
