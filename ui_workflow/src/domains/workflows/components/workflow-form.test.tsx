import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowForm } from "./workflow-form";
import type { WorkflowInput } from "@/domains/workflows/types";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: { useAppHeaderSlot: () => null },
}));
vi.mock("./workflow-graph", () => ({
  WorkflowGraph: () => <div aria-label="graph" />,
}));
afterEach(cleanup);

const draft: WorkflowInput = {
  name: "Video",
  scope: "media",
  output_step_key: "",
  steps: [],
};
const option = {
  id: "template-a",
  name: "Portrait",
  version: 3,
  status: "published",
};

describe("workflow template picker", () => {
  it("opens a picker without automatically adding the first template", () => {
    const onChange = vi.fn();
    render(
      <WorkflowForm
        value={draft}
        onChange={onChange}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        submitLabel="Save"
        isPending={false}
        templateOptions={[option]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "editor.addNode" }));
    expect(
      screen.getByRole("dialog", { name: "studio.chooseTemplate" }),
    ).toBeTruthy();
    expect(onChange).not.toHaveBeenCalled();
  });
  it("reads the selected frozen version before creating bindings", async () => {
    const onChange = vi.fn();
    const resolveTemplate = vi.fn().mockResolvedValue({
      ...option,
      inputs: [{ name: "person", required: true }],
    });
    render(
      <WorkflowForm
        value={draft}
        onChange={onChange}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        submitLabel="Save"
        isPending={false}
        templateOptions={[option]}
        resolveTemplate={resolveTemplate}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "editor.addNode" }));
    fireEvent.click(screen.getByRole("button", { name: /Portrait/ }));
    await waitFor(() => expect(onChange).toHaveBeenCalledOnce());
    expect(resolveTemplate).toHaveBeenCalledWith(option);
    expect(onChange.mock.calls[0][0].steps[0]).toMatchObject({
      template_id: "template-a",
      template_version: 3,
      slot_bindings: [
        { to_input: "person", source: "workflow_input", ref: "person" },
      ],
    });
  });
  it("leaves the draft untouched when the version read fails or returns a different version", async () => {
    const onChange = vi.fn();
    const resolveTemplate = vi
      .fn()
      .mockRejectedValueOnce(new Error("Unavailable"))
      .mockResolvedValueOnce({ ...option, version: 4, inputs: [] });
    render(
      <WorkflowForm
        value={draft}
        onChange={onChange}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        submitLabel="Save"
        isPending={false}
        templateOptions={[option]}
        resolveTemplate={resolveTemplate}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "editor.addNode" }));
    fireEvent.click(screen.getByRole("button", { name: /Portrait/ }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("Unavailable"),
    );
    fireEvent.click(screen.getByRole("button", { name: /Portrait/ }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe(
        "studio.templateContractError",
      ),
    );
    expect(onChange).not.toHaveBeenCalled();
  });
});
