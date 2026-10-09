import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { InvocationMappingEditor } from "./invocation-mapping-editor";
import type { InvocationExpression } from "@/domains/templates/types";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      values?.path
        ? `${key}:${values.path}`
        : values?.name
          ? `${key}:${values.name}`
          : key,
  }),
}));
afterEach(cleanup);

describe("structured invocation mapping", () => {
  it("reorders references while retaining other parameters and lets users edit a slot", () => {
    const onChange = vi.fn();
    function Form() {
      const [value, setValue] = useState<Record<string, InvocationExpression>>({
        prompt: { op: "render_template" },
        images: {
          op: "list",
          items: [
            { op: "asset_ref", slot: "person" },
            { op: "asset_ref", slot: "product" },
          ],
        },
      });
      return (
        <InvocationMappingEditor
          value={value}
          slots={["person", "product"].map((name) => ({
            name,
            asset_kind: "image",
            resource_type: "ASSET",
            required: true,
          }))}
          onChange={(next) => {
            setValue(next);
            onChange(next);
          }}
        />
      );
    }
    render(<Form />);
    fireEvent.click(
      screen.getByRole("button", { name: "mapping.up:images[1]" }),
    );
    expect(
      onChange.mock.calls[0][0].images.items.map(
        (item: InvocationExpression) => item.slot,
      ),
    ).toEqual(["product", "person"]);
    expect(onChange.mock.calls[0][0].prompt).toEqual({ op: "render_template" });
    fireEvent.change(
      screen.getByRole("combobox", { name: "mapping.slot:images[0]" }),
      { target: { value: "person" } },
    );
    expect(onChange.mock.lastCall?.[0].images.items[0].slot).toBe("person");
  });
  it("does not overwrite a parameter when its name is reused", () => {
    const onChange = vi.fn();
    render(
      <InvocationMappingEditor
        value={{ prompt: { op: "render_template" } }}
        slots={[]}
        onChange={onChange}
      />,
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: "mapping.parameterName" }),
      { target: { value: " prompt " } },
    );
    expect(
      (
        screen.getByRole("button", {
          name: "mapping.addParameter",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(onChange).not.toHaveBeenCalled();
  });
});
