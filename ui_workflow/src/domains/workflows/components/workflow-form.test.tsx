import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
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
  it("allows choosing an existing node as final output and keeps it after confirmation", async () => {
    const scrollIntoView = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "scrollIntoView",
    );
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: vi.fn(),
    });
    try {
      const submit = vi.fn();
      function Editor() {
        const [value, setValue] = useState<WorkflowInput>({
          ...draft,
          name: "",
          output_step_key: "first",
          steps: [
            { key: "first", template_id: "template-a", slot_bindings: [] },
            { key: "last", template_id: "template-a", slot_bindings: [] },
          ],
        });
        return (
          <WorkflowForm
            value={value}
            onChange={setValue}
            onSubmit={submit}
            onCancel={vi.fn()}
            submitLabel="Save"
            isPending={false}
          />
        );
      }
      render(<Editor />);
      fireEvent.keyDown(
        screen.getByRole("combobox", { name: "editorUI.output" }),
        { key: "ArrowDown" },
      );
      fireEvent.click(await screen.findByRole("option", { name: "last" }));
      fireEvent.click(
        screen.getByRole("button", { name: "studio.confirmSettings" }),
      );
      fireEvent.click(
        screen.getByRole("button", { name: "editorUI.settings" }),
      );
      expect(
        screen.getByRole("combobox", { name: "editorUI.output" }).textContent,
      ).toContain("last");
      expect(submit).not.toHaveBeenCalled();
    } finally {
      if (scrollIntoView)
        Object.defineProperty(
          HTMLElement.prototype,
          "scrollIntoView",
          scrollIntoView,
        );
      else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
    }
  });
  it("explains empty output choices and confirms settings without submitting or losing the draft", () => {
    const submit = vi.fn();
    function Editor() {
      const [value, setValue] = useState({ ...draft, name: "" });
      return (
        <WorkflowForm
          value={value}
          onChange={setValue}
          onSubmit={submit}
          onCancel={vi.fn()}
          submitLabel="Save"
          isPending={false}
        />
      );
    }
    render(<Editor />);
    expect(screen.getByText("studio.outputEmpty")).toBeTruthy();
    expect(
      (
        screen.getByRole("combobox", {
          name: "editorUI.output",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    fireEvent.change(screen.getByLabelText("editorUI.name"), {
      target: { value: "My workflow" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "studio.confirmSettings" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "editorUI.settings" }));
    expect(
      (screen.getByLabelText("editorUI.name") as HTMLInputElement).value,
    ).toBe("My workflow");
    expect(submit).not.toHaveBeenCalled();
  });
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
