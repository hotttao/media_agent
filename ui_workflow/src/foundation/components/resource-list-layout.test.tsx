import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ListPagination, ListState } from "./resource-list-layout";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
describe("list recovery and pagination", () => {
  it("uses the total record count, including the final partial page", () => {
    const change = vi.fn();
    render(
      <ListPagination page={3} pageSize={20} total={41} onPage={change} />,
    );
    expect(
      (screen.getByRole("button", { name: "common.next" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "common.previous" }));
    expect(change).toHaveBeenCalledWith(2);
  });
  it("announces failures and provides retry rather than claiming the list is empty", () => {
    const retry = vi.fn();
    render(
      <ListState
        error="Permission source unavailable"
        empty="No records"
        onRetry={retry}
      />,
    );
    expect(screen.getByRole("alert").textContent).toBe(
      "Permission source unavailable",
    );
    expect(screen.queryByText("No records")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "v12.retry" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
