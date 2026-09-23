import { fireEvent, render, screen } from "@testing-library/react";
import type { FormEvent } from "react";
import { describe, expect, it, vi } from "vitest";
import { PasswordInput } from "@/components/ui/password-input";

describe("PasswordInput", () => {
  it("toggles visibility without changing the value", () => {
    render(<PasswordInput aria-label="Password" defaultValue="secret" />);

    const input = screen.getByLabelText("Password") as HTMLInputElement;
    expect(input.type).toBe("password");

    fireEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(input.type).toBe("text");
    expect(input.value).toBe("secret");
    expect(
      screen
        .getByRole("button", { name: "Hide password" })
        .getAttribute("aria-pressed"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Hide password" }));
    expect(input.type).toBe("password");
    expect(input.value).toBe("secret");
  });

  it("does not submit the surrounding form", () => {
    const onSubmit = vi.fn((event: FormEvent) => event.preventDefault());
    render(
      <form onSubmit={onSubmit}>
        <PasswordInput aria-label="Password" />
      </form>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("disables both controls", () => {
    render(<PasswordInput aria-label="Password" disabled />);

    expect(
      (screen.getByLabelText("Password") as HTMLInputElement).disabled,
    ).toBe(true);
    expect(
      (
        screen.getByRole("button", {
          name: "Show password",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });
});
