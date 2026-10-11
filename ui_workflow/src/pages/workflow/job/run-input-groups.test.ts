import { describe, expect, it } from "vitest";
import { runInputGroups } from "./run-input-groups";
import type { WorkflowStep } from "@/domains/workflows/types";

const entry = {
  name: "product",
  resource_type: "PRODUCT" as const,
  required: true,
};
const a: WorkflowStep = {
  key: "a",
  template_id: "tpl",
  slot_bindings: [
    { source: "workflow_input", ref: "product", to_input: "garment" },
  ],
};
const b: WorkflowStep = {
  key: "b",
  template_id: "tpl",
  slot_bindings: [
    { source: "step_output", ref: "a", to_input: "image" },
    { source: "workflow_input", ref: "product", to_input: "garment" },
  ],
};
describe("run preparation groups", () => {
  it("sorts by real dependencies and asks shared input only at its first consumer", () => {
    const result = runInputGroups([b, a], [entry]);
    expect(result.steps.map((step) => step.key)).toEqual(["a", "b"]);
    expect(result.groups).toEqual([{ stepKey: "a", entries: [entry] }]);
  });
  it("skips constant-only nodes and preserves stable independent node order", () => {
    const constant: WorkflowStep = {
      key: "constant",
      template_id: "tpl",
      slot_bindings: [{ source: "const", to_input: "x", const_value: 1 }],
    };
    expect(runInputGroups([constant, a], [entry]).groups[0].stepKey).toBe("a");
    expect(runInputGroups([constant], []).groups).toEqual([]);
  });
  it("rejects cycles, unknown predecessors and missing input contracts", () => {
    expect(() =>
      runInputGroups(
        [
          {
            ...a,
            slot_bindings: [{ source: "step_output", ref: "b", to_input: "x" }],
          },
          b,
        ],
        [entry],
      ),
    ).toThrow();
    expect(() => runInputGroups([b], [entry])).toThrow();
    expect(() => runInputGroups([a], [])).toThrow();
    expect(() => runInputGroups([], [entry])).toThrow();
  });
});
