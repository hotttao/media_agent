import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workflow } from "@/domains/workflows/types";
import type { JobInputOptions } from "./use-job-input-options";
import { RunWizard } from "./run-wizard";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, args?: { name?: string }) =>
      args?.name ? `${key} ${args.name}` : key,
  }),
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useAppHeaderSlot: () => null,
    useInstanceState: (_id: string, _key: string, initial: () => unknown) =>
      useState(initial),
  },
}));
vi.mock("@/domains/workflows/components/workflow-graph", () => ({
  WorkflowGraph: ({ selectedNodeId }: { selectedNodeId?: string }) => (
    <div data-testid="active-node">{selectedNodeId}</div>
  ),
}));
vi.mock("@/foundation/components/library-media", () => ({
  LibraryMedia: ({ alt }: { alt: string }) => <img alt={alt} src="/test.png" />,
}));
afterEach(cleanup);
const workflow = {
  id: "wf",
  name: "Video",
  version: 1,
  output_step_key: "video",
  steps: [
    {
      key: "image",
      template_id: "tpl",
      slot_bindings: [
        { source: "workflow_input", to_input: "product", ref: "product" },
      ],
    },
    {
      key: "video",
      template_id: "tpl",
      slot_bindings: [
        { source: "step_output", to_input: "image", ref: "image" },
        { source: "workflow_input", to_input: "movement", ref: "movement" },
      ],
    },
  ],
} as Workflow;
const entries = [
  { name: "product", resource_type: "PRODUCT" as const, required: true },
  { name: "movement", resource_type: "MATERIAL" as const, required: true },
];
const options = {
  product: [
    {
      id: "p1",
      name: "Red dress",
      scope: "media",
      images: [{ object_path: "dress.png", is_main: true }],
    },
    { id: "p2", name: "Blue shirt", scope: "media", images: [] },
  ],
  movement: [
    {
      id: "m1",
      name: "Walk",
      content: "Walk towards the camera",
      scope: "media",
      asset_kind: "movement",
    },
  ],
} as unknown as JobInputOptions;

describe("canvas run preparation", () => {
  it("searches visual choices, requires selection, advances nodes, preserves choices on back and submits only at review", () => {
    const submit = vi.fn();
    function Screen() {
      const [selected, setSelected] = useState<Record<string, string>>({});
      const [workspace, setWorkspace] = useState("");
      return (
        <RunWizard
          instanceId="instance"
          organizationId="org"
          launchKey="intent"
          workflow={workflow}
          entries={entries}
          options={options}
          selected={selected}
          workspaceId={workspace}
          workspaces={[{ id: "ws", name: "Studio" }]}
          onChoose={(name, id) =>
            setSelected((old) => ({ ...old, [name]: id }))
          }
          onWorkspace={setWorkspace}
          onSubmit={submit}
          onCancel={vi.fn()}
          onRetry={vi.fn()}
          busy={false}
          loading={false}
          replay={false}
          canSubmit={Boolean(workspace)}
        />
      );
    }
    render(<Screen />);
    expect(screen.getByTestId("active-node").textContent).toBe("image");
    expect(
      (
        screen.getByRole("button", {
          name: "runWizard.next",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(screen.getByRole("img", { name: "Red dress" })).toBeTruthy();
    fireEvent.change(
      screen.getByRole("textbox", {
        name: "runWizard.search runForm.fields.product",
      }),
      { target: { value: "Red" } },
    );
    expect(screen.queryByRole("button", { name: "Blue shirt" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Red dress" }));
    fireEvent.click(screen.getByRole("button", { name: "runWizard.next" }));
    expect(screen.getByTestId("active-node").textContent).toBe("video");
    fireEvent.click(screen.getByRole("button", { name: "runWizard.previous" }));
    expect(
      screen
        .getByRole("button", { name: "Red dress" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "runWizard.next" }));
    fireEvent.change(
      screen.getByRole("textbox", {
        name: "runWizard.search runForm.fields.movement",
      }),
      { target: { value: "camera" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Walk" }));
    fireEvent.click(screen.getByRole("button", { name: "runWizard.next" }));
    expect(submit).not.toHaveBeenCalled();
    expect(screen.getByText("runWizard.review")).toBeTruthy();
    fireEvent.change(
      screen.getByRole("combobox", { name: "editorContext.workspace" }),
      { target: { value: "ws" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "runForm.submit" }));
    expect(submit).toHaveBeenCalledOnce();
  }, 20_000);
});
