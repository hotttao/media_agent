import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import { resumeJob, retryJob, submitJob } from "@/domains/jobs/lib/api";
import type { Job, StepRun } from "@/domains/jobs/types";
import type { Workflow } from "@/domains/workflows/types";
import { useJobInputOptions } from "./use-job-input-options";
import { buildJobInput } from "./job-input";
import { JobCreate } from "./create";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useInstanceState: (_id: string, _key: string, initial: () => unknown) =>
      useState(initial),
    useShellContext: () => ({ organization: { id: "org" }, isLoading: false }),
    useAppHeaderSlot: () => null,
  },
}));
vi.mock("@/domains/workflows/hooks/use-workflow", () => ({
  useWorkflow: vi.fn(),
}));
vi.mock("@/domains/jobs/hooks/use-workflows-list", () => ({
  useWorkflowsList: () => ({
    data: [{ id: "wf", name: "Current head", version: 9 }],
  }),
}));
vi.mock("./use-job-input-options", () => ({ useJobInputOptions: vi.fn() }));
vi.mock("@/domains/jobs/lib/api", () => ({
  resumeJob: vi.fn(),
  retryJob: vi.fn(),
  submitJob: vi.fn(),
}));
vi.mock("./job-input", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./job-input")>()),
  buildJobInput: vi.fn(),
}));

const source = {
  id: "job",
  workflow_id: "wf",
  workflow_version: 2,
  organization_id: "org",
  workspace_id: "workspace",
  scope: "original-scope",
  status: "awaiting_resume",
  origin: "legacy_import",
  input: { product: { id: "existing-product" } },
} as unknown as Job;
const entries = [
  { name: "product", resource_type: "PRODUCT", required: true },
  { name: "movement", resource_type: "MATERIAL", required: true },
];
const frozen = {
  id: "wf",
  name: "Frozen",
  version: 2,
  scope: "",
  input_schema: { entries },
  steps: [
    {
      key: "image",
      template_id: "image-template",
      template_version: 1,
      slot_bindings: [
        { source: "workflow_input", to_input: "product", ref: "product" },
      ],
    },
    {
      key: "video",
      template_id: "video-template",
      template_version: 1,
      slot_bindings: [
        { source: "workflow_input", to_input: "movement", ref: "movement" },
      ],
    },
  ],
} as unknown as Workflow;
const props = {
  onDone: vi.fn(),
  onSubmitted: vi.fn(),
  instanceId: "test-slot",
  sourceJob: source,
  sourceStepRuns: [
    {
      job_id: "job",
      step_key: "image",
      template_id: "image-template",
      template_version: 1,
      origin: "legacy_import",
      status: "succeeded",
      output_asset_id: "original-image",
    },
    {
      job_id: "job",
      step_key: "video",
      template_id: "video-template",
      template_version: 1,
      origin: "native",
      status: "blocked",
    },
  ] as StepRun[],
};

function mount(
  mode: "resume" | "rerun" = "resume",
  sourceStepRuns = props.sourceStepRuns,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <JobCreate {...props} mode={mode} sourceStepRuns={sourceStepRuns} />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.mocked(useWorkflow).mockReturnValue({ data: frozen } as ReturnType<
    typeof useWorkflow
  >);
  vi.mocked(useJobInputOptions).mockReturnValue({
    data: {
      product: [{ id: "replacement", name: "Replacement" }],
      movement: [{ id: "movement", name: "Movement" }],
    },
    isSuccess: true,
  } as unknown as ReturnType<typeof useJobInputOptions>);
  vi.mocked(buildJobInput).mockResolvedValue({
    material: [{ name: "movement", content: "Turn" }],
  });
  vi.mocked(resumeJob).mockResolvedValue(source);
  vi.mocked(retryJob).mockResolvedValue(source);
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("frozen execution input forms", () => {
  it("does not ask for pose inputs belonging only to the restored historical image", () => {
    vi.mocked(useWorkflow).mockReturnValue({
      data: {
        ...frozen,
        input_schema: {
          entries: [
            ...entries,
            { name: "pose", resource_type: "MATERIAL", required: true },
          ],
        },
        steps: frozen.steps.map((s) =>
          s.key === "image"
            ? {
                ...s,
                slot_bindings: [
                  ...s.slot_bindings,
                  { source: "workflow_input" as const, to_input: "pose", ref: "pose" },
                ],
              }
            : s,
        ),
      },
    } as unknown as ReturnType<typeof useWorkflow>);
    mount();
    expect(screen.queryByTestId("run-input-pose")).toBeNull();
    expect(screen.getByTestId("run-input-movement")).toBeTruthy();
  });
  it("blocks Resume without its retained StepRun evidence", () => {
    mount("resume", []);
    expect(screen.getByText("runForm.frozenUnavailable")).toBeTruthy();
    expect(
      (
        screen.getByRole("button", {
          name: "runForm.submit",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(resumeJob).not.toHaveBeenCalled();
  });
  it("requests the Job version and only asks for missing inputs on Resume", () => {
    mount();
    expect(useWorkflow).toHaveBeenCalledWith("wf", 2);
    expect(screen.queryByTestId("run-input-product")).toBeNull();
    expect(screen.getByTestId("run-input-movement")).toBeTruthy();
    expect(useJobInputOptions).toHaveBeenCalledWith(
      expect.objectContaining({
        version: 2,
        scope: "original-scope",
        input_schema: { entries: [entries[1]] },
      }),
      true,
      "org",
    );
  });
  it("submits only the missing bundle to the original Job with a retained request key", async () => {
    mount();
    fireEvent.change(screen.getByTestId("run-input-movement"), {
      target: { value: "movement" },
    });
    fireEvent.click(screen.getByRole("button", { name: "runForm.submit" }));
    await waitFor(() => expect(props.onSubmitted).toHaveBeenCalledWith("job"));
    expect(resumeJob).toHaveBeenCalledWith(
      "job",
      { material: [{ name: "movement", content: "Turn" }] },
      expect.any(String),
    );
    expect(submitJob).not.toHaveBeenCalled();
    expect(retryJob).not.toHaveBeenCalled();
  });
  it("reruns the original Job with input_override rather than submitting a new Job", async () => {
    mount("rerun");
    fireEvent.change(screen.getByTestId("run-input-product"), {
      target: { value: "replacement" },
    });
    fireEvent.change(screen.getByTestId("run-input-movement"), {
      target: { value: "movement" },
    });
    fireEvent.click(screen.getByRole("button", { name: "runForm.submit" }));
    await waitFor(() =>
      expect(retryJob).toHaveBeenCalledWith("job", {
        input_override: { material: [{ name: "movement", content: "Turn" }] },
      }),
    );
    expect(submitJob).not.toHaveBeenCalled();
    expect(resumeJob).not.toHaveBeenCalled();
  });
  it("replays an uncertain Resume with the same bundle and request key", async () => {
    vi.mocked(resumeJob).mockRejectedValueOnce(new Error("Response lost"));
    mount();
    fireEvent.change(screen.getByTestId("run-input-movement"), {
      target: { value: "movement" },
    });
    fireEvent.click(screen.getByRole("button", { name: "runForm.submit" }));
    await waitFor(() => expect(screen.getByText("Response lost")).toBeTruthy());
    const original = vi.mocked(resumeJob).mock.calls[0];
    fireEvent.click(screen.getByRole("button", { name: "runForm.replay" }));
    await waitFor(() => expect(resumeJob).toHaveBeenCalledTimes(2));
    expect(vi.mocked(resumeJob).mock.calls[1]).toEqual(original);
    expect(buildJobInput).toHaveBeenCalledTimes(1);
    expect(submitJob).not.toHaveBeenCalled();
  });
  it("does not automatically POST a rerun again after an uncertain response", async () => {
    vi.mocked(retryJob).mockRejectedValueOnce(new Error("Response lost"));
    mount("rerun");
    fireEvent.change(screen.getByTestId("run-input-product"), {
      target: { value: "replacement" },
    });
    fireEvent.change(screen.getByTestId("run-input-movement"), {
      target: { value: "movement" },
    });
    fireEvent.click(screen.getByRole("button", { name: "runForm.submit" }));
    await waitFor(() =>
      expect(screen.getByText("runForm.rerunUnknown")).toBeTruthy(),
    );
    const retry = screen.getByRole("button", {
      name: "runForm.replay",
    }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    fireEvent.click(retry);
    expect(retryJob).toHaveBeenCalledTimes(1);
    expect(submitJob).not.toHaveBeenCalled();
  });
  it("blocks a different version instead of falling back to the current head", () => {
    vi.mocked(useWorkflow).mockReturnValue({
      data: { ...frozen, version: 9 },
    } as ReturnType<typeof useWorkflow>);
    mount("rerun");
    expect(screen.getByText("runForm.frozenUnavailable")).toBeTruthy();
    expect(screen.queryByTestId("run-input-product")).toBeNull();
    expect(
      (
        screen.getByRole("button", {
          name: "runForm.submit",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(useJobInputOptions).toHaveBeenCalledWith(
      expect.anything(),
      false,
      "org",
    );
  });
  it("blocks missing frozen evidence even if the current head is listed", () => {
    vi.mocked(useWorkflow).mockReturnValue({
      data: undefined,
      isError: true,
      error: new Error("Version unavailable"),
    } as ReturnType<typeof useWorkflow>);
    mount();
    expect(screen.getByText("runForm.frozenUnavailable")).toBeTruthy();
    expect(
      (
        screen.getByRole("button", {
          name: "runForm.submit",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(submitJob).not.toHaveBeenCalled();
  });
});
