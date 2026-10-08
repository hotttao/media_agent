import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PasswordField } from "./password-field";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
describe("password visibility", () => {
  it("toggles without changing the password or submitting the form", () => {
    const submit = vi.fn((event) => event.preventDefault());
    render(
      <form onSubmit={submit}>
        <label htmlFor="password">Password</label>
        <PasswordField id="password" defaultValue="test-value" />
      </form>,
    );
    const field = screen.getByLabelText("Password") as HTMLInputElement;
    expect(field.type).toBe("password");
    fireEvent.click(screen.getByRole("button", { name: "auth.showPassword" }));
    expect(field.type).toBe("text");
    expect(field.value).toBe("test-value");
    expect(submit).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "auth.hidePassword" }));
    expect(field.type).toBe("password");
  });
  it("disables the visibility control while submitting", () => {
    render(<PasswordField disabled />);
    expect(
      (
        screen.getByRole("button", {
          name: "auth.showPassword",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });
});
