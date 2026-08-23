import { expect, test } from "../fixtures/base";

const admin = {
  id: "admin-1",
  username: "admin",
  role: "admin",
  auth_source: "password",
  default_prompt_id: "",
  auto_evolution: false,
};
const target = {
  id: "user-1",
  username: "alice",
  display_name: "Alice",
  email: "alice@example.test",
  role: "user",
  auth_source: "password",
  created_at: "2026-08-23T00:00:00Z",
  status: "active",
  auth_version: 1,
};

test.describe("user management", () => {
  test("admin creates a user and soft-deletes an active user", {
    tag: "@C000010",
  }, async ({ page }) => {
    await page.route("**/api/auth/me", (route) =>
      route.fulfill({ json: admin }),
    );
    let users = [target];
    await page.route("**/api/users?*", (route) =>
      route.fulfill({
        json: { items: users, total: users.length, page: 1, page_size: 100 },
      }),
    );
    await page.route("**/api/users", async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({
        username: "bob",
        role: "user",
      });
      users = [
        ...users,
        { ...target, id: "user-2", username: "bob", display_name: "Bob" },
      ];
      await route.fulfill({ status: 201, json: users[1] });
    });
    await page.route("**/api/users/user-1", async (route) => {
      expect(route.request().method()).toBe("DELETE");
      users = users.map((item) =>
        item.id === "user-1"
          ? { ...item, status: "deleted" as const, auth_version: 2 }
          : item,
      );
      await route.fulfill({ json: { success: true } });
    });

    await page.goto("/admin/users");
    await expect(
      page.getByRole("heading", { name: "User management" }),
    ).toBeVisible();
    await expect(page.getByTestId("users-table")).toContainText(
      "alice@example.test",
    );

    await page.getByRole("button", { name: "Create user" }).click();
    await page
      .getByRole("dialog", { name: "Create user" })
      .getByLabel("Username")
      .fill("bob");
    await page
      .getByRole("dialog", { name: "Create user" })
      .getByLabel("Display name")
      .fill("Bob");
    await page
      .getByRole("dialog", { name: "Create user" })
      .getByLabel("Password")
      .fill("password123");
    await page
      .getByRole("dialog", { name: "Create user" })
      .getByRole("button", { name: "Create user" })
      .click();
    await expect(page.getByTestId("users-table")).toContainText("bob");

    await page.getByRole("button", { name: "Delete alice" }).click();
    await page
      .getByRole("dialog", { name: "Revoke user access" })
      .getByRole("button", { name: "Revoke access" })
      .click();
    await expect(page.getByTestId("users-table")).toContainText("Deleted");
  });

  test("non-admin cannot open the management route", {
    tag: "@C000011",
  }, async ({ page }) => {
    await page.route("**/api/auth/me", (route) =>
      route.fulfill({ json: { ...admin, role: "user" } }),
    );
    await page.route("**/api/v1/workspaces", (route) =>
      route.fulfill({ json: { workspaces: [] } }),
    );
    await page.goto("/admin/users");
    await expect(page).toHaveURL(/\/$/);
  });
});
