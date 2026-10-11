import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useJob } from "@/domains/jobs/hooks/use-job";
import { useGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import type { GetJobResponse } from "@/domains/jobs/types";
import type { Workflow } from "@/domains/workflows/types";
import { JobShow } from "./show";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useInstanceState: (_id: string, _key: string, initial: () => unknown) =>
      useState(initial),
    useShellContext: () => ({ organization: { id: "org" } }),
    useAppHeaderSlot: () => null,
  },
}));
vi.mock("@/domains/jobs/hooks/use-job", () => ({ useJob: vi.fn() }));
vi.mock("@/domains/workflows/hooks/use-workflow", () => ({
  useWorkflow: vi.fn(),
}));
vi.mock("@/domains/generated-assets/hooks/use-generated-asset", () => ({
  useGeneratedAsset: vi.fn(() => ({ data: undefined })),
}));
vi.mock("@/domains/jobs/hooks/use-job-mutations", () => ({
  useCancelJob: () => ({ isPending: false }),
  useRetryJob: () => ({ isPending: false }),
  useConfirmStepRun: () => ({ isPending: false }),
  useRegenerateStepRun: () => ({ isPending: false }),
}));
// FlowGram needs a browser viewport. Exercise the page's selection contract,
// while real FlowGram node rendering and container layout are covered by E2E.
vi.mock("@/domains/workflows/components/workflow-graph", () => ({
  WorkflowGraph: ({
    model,
    onNodeSelect,
  }: {
    model: { steps: { key: string }[] };
    onNodeSelect: (key: string) => void;
  }) => (
    <div data-testid="workflow-canvas">
      {model.steps.map((step) => (
        <button
          type="button"
          key={step.key}
          onClick={() => onNodeSelect(step.key)}
        >
          {step.key}
        </button>
      ))}
    </div>
  ),
}));

const detail = {
  job: {
    id: "job",
    workflow_id: "wf",
    workflow_version: 1,
    status: "need_confirm",
    scope: "test",
    input: {},
    submitted_at: "",
    retry_history: [],
  },
  step_runs: [
    {
      id: "image-run",
      step_key: "image",
      step_order: 0,
      status: "need_confirm",
      template_version: 1,
      candidates: [{ index: 0, url: "", content: "Image candidate" }],
      resolved_inputs: {},
    },
  ],
} as unknown as GetJobResponse;
const workflow = {
  id: "wf",
  version: 1,
  name: "Test workflow",
  output_step_key: "video",
  steps: [
    { key: "image", slot_bindings: [] },
    { key: "video", slot_bindings: [] },
  ],
} as unknown as Workflow;
function setDetail(value = detail) {
  vi.mocked(useJob).mockReturnValue({
    data: value,
    isLoading: false,
    isError: false,
  } as ReturnType<typeof useJob>);
}
beforeEach(() => {
  setDetail();
  vi.mocked(useGeneratedAsset).mockReturnValue({ data: undefined } as ReturnType<typeof useGeneratedAsset>);
  vi.mocked(useWorkflow).mockReturnValue({ data: workflow } as ReturnType<
    typeof useWorkflow
  >);
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
const props = {
  id: "job",
  instanceId: "window",
  onBack: vi.fn(),
  onResume: vi.fn(),
  onRerun: vi.fn(),
};

describe("execution inspector", () => {
  it("shows an accepted text result without inventing candidates or an empty result", () => {
    setDetail({ ...detail, step_runs: [{ ...detail.step_runs[0], status: "succeeded", candidates: [], output_asset_id: "gen_reused", result_source: "reused", reused_from_step_run_id: "source" }] });
    vi.mocked(useGeneratedAsset).mockReturnValue({ data: { content: "Accepted text from prior execution" } } as ReturnType<typeof useGeneratedAsset>);
    render(<JobShow {...props} />);
    fireEvent.click(screen.getByRole("button", { name: "image" }));
    expect(screen.getByText("Accepted text from prior execution")).toBeTruthy();
    expect(screen.getByText("execution.resultReused")).toBeTruthy();
    expect(screen.queryByText("execution.noResults")).toBeNull();
  });
  it("offers input override only for failed native executions, not success, cancellation or historical jobs", () => {
    setDetail({
      ...detail,
      job: { ...detail.job, status: "failed", origin: "native" },
    });
    const view = render(<JobShow {...props} />);
    fireEvent.click(screen.getByRole("button", { name: "runForm.rerun" }));
    expect(props.onRerun).toHaveBeenCalledTimes(1);
    for (const job of [
      { ...detail.job, status: "cancelled", origin: "native" },
      { ...detail.job, status: "succeeded", origin: "native" },
      { ...detail.job, status: "failed", origin: "legacy_import" },
    ] as GetJobResponse["job"][]) {
      setDetail({ ...detail, job });
      view.rerender(<JobShow {...props} />);
      expect(
        screen.queryByRole("button", { name: "runForm.rerun" }),
      ).toBeNull();
    }
  });
  it("shows the graph first and never substitutes another step's results for an unstarted node", () => {
    render(<JobShow {...props} />);
    expect(useWorkflow).toHaveBeenCalledWith("wf", 1);
    expect(screen.getByTestId("workflow-canvas")).toBeTruthy();
    expect(screen.queryByRole("complementary")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "video" }));
    expect(screen.getByText("execution.notStarted")).toBeTruthy();
    expect(screen.queryByText("Image candidate")).toBeNull();
  });
  it("closes with Escape, restores focus and remains closed after polling", () => {
    const view = render(<JobShow {...props} />);
    const node = screen.getByRole("button", { name: "image" });
    node.focus();
    fireEvent.click(node);
    expect(screen.getByText("Image candidate")).toBeTruthy();
    fireEvent.keyDown(
      screen.getByRole("button", { name: "execution.closeResults" }),
      { key: "Escape" },
    );
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(document.activeElement).toBe(node);
    setDetail({ ...detail, job: { ...detail.job, status: "running" } });
    view.rerender(<JobShow {...props} />);
    expect(screen.queryByRole("complementary")).toBeNull();
  });
  it("falls back to actual steps when the head definition differs from the execution version", () => {
    vi.mocked(useWorkflow).mockReturnValue({
      data: { ...workflow, version: 2 },
    } as ReturnType<typeof useWorkflow>);
    render(<JobShow {...props} />);
    expect(screen.queryByTestId("workflow-canvas")).toBeNull();
    expect(screen.getByText("execution.graphFallback")).toBeTruthy();
    fireEvent.click(
      within(screen.getByRole("navigation")).getByRole("button", {
        name: /image/,
      }),
    );
    expect(screen.getByText("Image candidate")).toBeTruthy();
  });
  it("locates pending review explicitly, but terminal parents have no approval action", () => {
    const view = render(<JobShow {...props} />);
    fireEvent.click(
      screen.getByRole("button", { name: "execution.inspectApproval" }),
    );
    expect(
      screen.getByRole("button", { name: "execution.confirm" }),
    ).toBeTruthy();
    setDetail({ ...detail, job: { ...detail.job, status: "cancelled" } });
    view.rerender(<JobShow {...props} />);
    expect(
      screen.queryByRole("button", { name: "execution.inspectApproval" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "execution.confirm" }),
    ).toBeNull();
    expect(screen.getByText("Image candidate")).toBeTruthy();
  });
});
