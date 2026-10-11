import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkflowLaunchIntent } from "@/foundation/types/app-navigation";
import { WorkflowApp } from "./app";

const fixture = vi.hoisted(() => ({
  incoming: null as WorkflowLaunchIntent | null,
  openProduct: vi.fn(),
  hasProductNavigation: true,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useAppHeaderSlot: () => null,
    useNavigationPort: () =>
      fixture.hasProductNavigation
        ? { openProduct: fixture.openProduct }
        : undefined,
    useInstanceState: (
      _instance: string,
      key: string,
      initial: () => unknown,
    ) =>
      useState(() =>
        key === "host_workflow_launch_intent" ? fixture.incoming : initial(),
      ),
    useInstancePersistentState: (
      _instance: string,
      _key: string,
      initial: () => unknown,
    ) => useState(initial),
  },
}));
vi.mock("@/domains/jobs/hooks/use-job", () => ({
  useJob: () => ({ data: undefined }),
}));
vi.mock("./workflow/list", () => ({
  WorkflowList: ({ onSelect }: { onSelect: (id: string) => void }) => (
    <button type="button" onClick={() => onSelect("workflow-a")}>
      Open workflow
    </button>
  ),
}));
vi.mock("./workflow/show", () => ({
  WorkflowShow: ({ id, onRun }: { id: string; onRun: () => void }) => (
    <>
      <p>Workflow {id}</p>
      <button type="button" onClick={onRun}>
        Run
      </button>
    </>
  ),
}));
vi.mock("./job/create", () => ({
  JobCreate: ({
    onDone,
    onSubmitted,
  }: {
    onDone: () => void;
    onSubmitted: (id: string) => void;
  }) => (
    <>
      <button type="button" onClick={() => onSubmitted("job-a")}>
        Confirm run
      </button>
      <button type="button" onClick={onDone}>
        Cancel run
      </button>
    </>
  ),
}));
vi.mock("./job/show", () => ({
  JobShow: ({ id, onBack }: { id: string; onBack: () => void }) => (
    <>
      <p>Execution {id}</p>
      <button type="button" onClick={onBack}>
        Back to workflow
      </button>
    </>
  ),
}));
vi.mock("./job/list", () => ({ JobList: () => <p>Job list</p> }));

afterEach(() => {
  cleanup();
  fixture.incoming = null;
  fixture.openProduct.mockReset();
  fixture.hasProductNavigation = true;
});

describe("cancel workflow run", () => {
  it("shows a submitted execution inside the workflow App and returns to its definition", () => {
    render(<WorkflowApp instanceId="workflow-instance" />);
    fireEvent.click(screen.getByRole("button", { name: "Open workflow" }));
    fireEvent.click(screen.getByRole("button", { name: "Run" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm run" }));
    expect(screen.getByText("Execution job-a")).toBeTruthy();
    expect(screen.queryByText("Job list")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to workflow" }));
    expect(screen.getByText("Workflow workflow-a")).toBeTruthy();
  });
  it("falls back to the source workflow when an older plugin has no product navigation", () => {
    fixture.hasProductNavigation = false;
    fixture.incoming = {
      kind: "workflow_run",
      requestId: "launch-old",
      organizationId: "org-a",
      workflowId: "workflow-a",
      productId: "product-a",
    };
    render(<WorkflowApp instanceId="workflow-instance" />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(screen.getByText("Workflow workflow-a")).toBeTruthy();
    expect(screen.queryByText("Job list")).toBeNull();
  });
  it("returns to the workflow that launched the run rather than the job list", () => {
    render(<WorkflowApp instanceId="workflow-instance" />);
    fireEvent.click(screen.getByRole("button", { name: "Open workflow" }));
    fireEvent.click(screen.getByRole("button", { name: "Run" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(screen.getByText("Workflow workflow-a")).toBeTruthy();
    expect(screen.queryByText("Job list")).toBeNull();
    expect(fixture.openProduct).not.toHaveBeenCalled();
  });

  it("explicitly returns to the source product regardless of the previous materials tab", () => {
    fixture.incoming = {
      kind: "workflow_run",
      requestId: "launch-a",
      organizationId: "org-a",
      workflowId: "workflow-a",
      productId: "product-a",
    };
    render(<WorkflowApp instanceId="workflow-instance" />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(fixture.openProduct).toHaveBeenCalledExactlyOnceWith({
      organizationId: "org-a",
      productId: "product-a",
    });
    expect(screen.getByText("Workflow workflow-a")).toBeTruthy();
  });
});
