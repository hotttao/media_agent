import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/foundation/providers/auth-provider", () => ({
  useAuth: vi.fn(),
}));

import { useAuth } from "@/foundation/providers/auth-provider";
import { SignOutButton } from "./sign-out-button";

describe("SignOutButton", () => {
  beforeEach(() => {
    vi.mocked(useAuth).mockReset();
  });

  it("starts the browser logout flow only once", async () => {
    const logout = vi.fn(() => new Promise<void>(() => {}));
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout,
      refreshUser: vi.fn(),
    });
    render(<SignOutButton />);

    const button = screen.getByRole("button", { name: "Sign out" });
    fireEvent.click(button);
    fireEvent.click(button);

    expect(logout).toHaveBeenCalledTimes(1);
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(button.textContent).toBe("Signing out…");
  });

  it("shows the logout failure and permits retry", async () => {
    const logout = vi.fn().mockRejectedValue(new Error("Logout unavailable"));
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout,
      refreshUser: vi.fn(),
    });
    render(<SignOutButton />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Logout unavailable",
    );
    await waitFor(() =>
      expect((screen.getByRole("button") as HTMLButtonElement).disabled).toBe(
        false,
      ),
    );
  });
});
