import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StructuredObjectEditor } from "./structured-object-editor";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
afterEach(cleanup);
describe("structured extension editing", () => {
  it("preserves nested values and types while updating one field", () => {
    const changed = vi.fn();
    function Form() {
      const [value, setValue] = useState<Record<string, unknown>>({
        name: "old",
        count: 2,
        enabled: true,
        nested: { note: "keep" },
        items: ["a", "b"],
      });
      return (
        <StructuredObjectEditor
          value={value}
          onChange={(next) => {
            setValue(next);
            changed(next);
          }}
        />
      );
    }
    render(<Form />);
    fireEvent.change(screen.getByRole("textbox", { name: "name" }), {
      target: { value: "new" },
    });
    expect(changed.mock.lastCall?.[0]).toEqual({
      name: "new",
      count: 2,
      enabled: true,
      nested: { note: "keep" },
      items: ["a", "b"],
    });
    fireEvent.change(screen.getByRole("spinbutton", { name: "count" }), {
      target: { value: "3" },
    });
    expect(changed.mock.lastCall?.[0].count).toBe(3);
  });
});
