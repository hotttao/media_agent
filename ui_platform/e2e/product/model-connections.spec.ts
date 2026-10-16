import { expect, test } from "@playwright/test";

test("personal auth.json import stays write-only and supports replace and revoke", async ({
  page,
}) => {
  await page.addInitScript(() => localStorage.setItem("tos-language", "en-US"));
  let record:
    | {
        id: string;
        name: string;
        kind: string;
        status: string;
        version: number;
      }
    | undefined;
  const writes: Record<string, unknown>[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me")
      return route.fulfill({
        json: {
          id: "owner",
          username: "owner",
          role: "user",
          auth_source: "password",
        },
      });
    if (
      path === "/api/v1/model-connections" &&
      route.request().method() === "GET"
    )
      return route.fulfill({ json: { connections: record ? [record] : [] } });
    if (
      path.startsWith("/api/v1/model-connections") &&
      route.request().method() !== "GET"
    ) {
      const body = route.request().postDataJSON();
      writes.push(body);
      record = {
        id: "connection",
        name: String(body.name ?? record?.name),
        kind: "codex_auth_json",
        status: path.endsWith("/revoke") ? "revoked" : "active",
        version: (record?.version ?? 0) + 1,
      };
      return route.fulfill({
        json: record,
        status:
          route.request().method() === "POST" && !path.endsWith("/revoke")
            ? 201
            : 200,
      });
    }
    return route.fulfill({ json: {} });
  });
  await page.goto("/model-connections");
  await page
    .getByLabel("Connection name", { exact: true })
    .fill("Personal Codex");
  const raw = JSON.stringify({
    tokens: {
      access_token: "private-access",
      refresh_token: "private-refresh",
    },
  });
  await page.getByLabel("auth.json", { exact: true }).setInputFiles({
    name: "auth.json",
    mimeType: "application/json",
    buffer: Buffer.from(raw),
  });
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Personal Codex", exact: true }),
  ).toBeVisible();
  expect(writes[0]?.auth_json).toBe(raw);
  await expect(page.getByLabel("auth.json", { exact: true })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText("private-access");
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
    "private-access",
  );
  await page
    .getByRole("button", { name: "Edit connection", exact: true })
    .click();
  await page
    .getByLabel("Connection name", { exact: true })
    .fill("Renamed Codex");
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Renamed Codex", exact: true }),
  ).toBeVisible();
  expect(writes[1]).toEqual({ name: "Renamed Codex", expected_version: 1 });
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(page.getByText("Revoked", { exact: false })).toBeVisible();
  expect(writes[2]).toEqual({ expected_version: 2 });
});
