import type { Page } from "@playwright/test";
import { expect, test } from "../fixtures/base";

const workspace = {
  id: "workspace-e2e",
  name: "E2E workspace",
  agent_id: "codex",
  agent_type: "codex",
  api_key_configured: true,
  configuration_version: 1,
  created_at: 1,
  updated_at: 1,
};

const session = {
  id: "session-e2e",
  workspace_id: workspace.id,
  agent_id: "codex",
  agent_type: "codex",
  title: "Session 1",
  status: "active",
  chat_status: "human",
  created_at: 1,
  updated_at: 1,
};

async function mockWorkspace(page: Page, sessions = [session]) {
  await page.route("**/api/v1/workspaces", (route) =>
    route.fulfill({ json: { workspaces: [workspace] } }),
  );
  await page.route(
    `**/api/chat/sessions?workspace_id=${workspace.id}`,
    (route) => route.fulfill({ json: { sessions } }),
  );
  await page.route("**/api/chat/sessions/*/messages", (route) =>
    route.fulfill({ json: { messages: [] } }),
  );
  await page.route("**/api/chat/sessions/*/events*", (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: "event: ready\ndata: {}\n\n",
    }),
  );
}

test.describe("workspace platform", () => {
  test("opens a workspace through the switcher and keeps it in the URL", {
    tag: "@C000001",
  }, async ({ page }) => {
    await mockWorkspace(page);
    await page.goto("/");

    await expect(
      page.getByRole("heading", { name: "Choose a workspace" }),
    ).toBeVisible();
    await page.getByLabel("Switch workspace").click();
    await page.getByRole("button", { name: /E2E workspace/ }).click();

    await expect(page).toHaveURL(/\/w\/workspace-e2e\?session=session-e2e$/);
    await expect(page.getByTestId("session-list")).toContainText("Session 1");
    await expect(page.getByLabel("Message Codex")).toBeEnabled();
  });

  test("uses one active slot on mobile and switches it from the dock", {
    tag: "@C000005",
  }, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await mockWorkspace(page);
    await page.goto(`/w/${workspace.id}`);

    await expect(page.getByTestId("slot-slot-c")).toBeVisible();
    await expect(page.getByTestId("slot-slot-a")).toHaveCount(0);
    await page.getByRole("button", { name: "Sessions", exact: true }).click();
    await expect(page.getByTestId("slot-slot-a")).toBeVisible();
    await expect(page.getByTestId("slot-slot-c")).toHaveCount(0);
    await page.getByRole("button", { name: "Chat", exact: true }).click();
    await expect(page.getByTestId("slot-slot-c")).toBeVisible();
  });

  test("updates workspace configuration from the switcher dialog", {
    tag: "@C000004",
  }, async ({ page }) => {
    await mockWorkspace(page, []);
    await page.route(`**/api/v1/workspaces/${workspace.id}`, async (route) => {
      expect(route.request().method()).toBe("PUT");
      expect(route.request().postDataJSON()).toEqual({
        name: "Updated workspace",
        agent_type: "codex",
        api_key: "replacement-key",
      });
      await route.fulfill({
        json: {
          ...workspace,
          name: "Updated workspace",
          configuration_version: 2,
        },
      });
    });
    await page.goto(`/w/${workspace.id}`);

    await page.getByLabel("Switch workspace").click();
    await page.getByRole("button", { name: "Workspace settings" }).click();
    await page.getByLabel("Settings workspace name").fill("Updated workspace");
    await page.getByLabel("Settings API key").fill("replacement-key");
    await page.getByRole("button", { name: "Save changes" }).click();

    await expect(page.getByLabel("Switch workspace")).toContainText(
      "Updated workspace",
    );
  });

  test("creates a workspace and navigates to its stable route", {
    tag: "@C000003",
  }, async ({ page }) => {
    await page.route("**/api/v1/workspaces", async (route) => {
      if (route.request().method() === "POST") {
        expect(route.request().postDataJSON()).toEqual({
          name: "Creator workspace",
          agent_type: "claude-code",
          api_key: "test-key",
        });
        await route.fulfill({
          status: 201,
          json: {
            ...workspace,
            id: "workspace-new",
            name: "Creator workspace",
            agent_id: "claude-code",
            agent_type: "claude-code",
          },
        });
      } else await route.fulfill({ json: { workspaces: [] } });
    });
    await page.route(
      "**/api/chat/sessions?workspace_id=workspace-new",
      (route) => route.fulfill({ json: { sessions: [] } }),
    );
    await page.goto("/");

    await page.getByRole("button", { name: "New workspace" }).click();
    await page.getByLabel("Workspace name").fill("Creator workspace");
    await page.getByLabel("Agent").selectOption("claude-code");
    await page.getByLabel("API key").fill("test-key");
    await page.getByRole("button", { name: "Create workspace" }).click();

    await expect(page).toHaveURL(/\/w\/workspace-new$/);
    await expect(page.getByLabel("Switch workspace")).toContainText(
      "Creator workspace",
    );
  });

  test("fills and restores an app window without losing the workspace", {
    tag: "@C000006",
  }, async ({ page }) => {
    await mockWorkspace(page);
    await page.goto(`/w/${workspace.id}`);

    await page.getByTestId("slot-slot-c").getByLabel("Fill slot").click();
    await expect(page.getByTestId("slot-slot-c")).toBeVisible();
    await expect(page.getByTestId("slot-slot-a")).toHaveCount(0);
    await page.getByLabel("Restore layout").click();
    await expect(page.getByTestId("slot-slot-a")).toBeVisible();
    await expect(page.getByTestId("slot-slot-b")).toBeVisible();
  });

  test("does not render stale history after a rapid session switch", {
    tag: "@C000007",
  }, async ({ page }) => {
    const second = { ...session, id: "session-second", title: "Session 2" };
    await mockWorkspace(page, [session, second]);
    let firstRequests = 0;
    await page.route(
      `**/api/chat/sessions/${session.id}/messages`,
      async (route) => {
        firstRequests += 1;
        if (firstRequests > 1)
          await new Promise((resolve) => setTimeout(resolve, 250));
        await route.fulfill({
          json: {
            messages:
              firstRequests > 1
                ? [
                    {
                      id: "stale",
                      turn_id: "t1",
                      role: "assistant",
                      content_json: '[{"type":"text","text":"Stale answer"}]',
                      sequence: 1,
                      created_at: 1,
                    },
                  ]
                : [],
          },
        });
      },
    );
    await page.route(`**/api/chat/sessions/${second.id}/messages`, (route) =>
      route.fulfill({ json: { messages: [] } }),
    );
    await page.goto(`/w/${workspace.id}`);

    await page.getByRole("button", { name: "Session 1" }).click();
    await page.getByRole("button", { name: "Session 2" }).click();

    await expect(page).toHaveURL(/session=session-second$/);
    await expect(page.getByTestId("chat-messages")).not.toContainText(
      "Stale answer",
    );
  });

  test("stops a running agent without changing status optimistically", {
    tag: "@C000008",
  }, async ({ page }) => {
    const running = { ...session, chat_status: "agent" };
    await mockWorkspace(page, [running]);
    let interruptRequests = 0;
    await page.route(
      `**/api/chat/sessions/${session.id}/interrupt`,
      async (route) => {
        interruptRequests += 1;
        expect(route.request().method()).toBe("POST");
        await route.fulfill({
          status: 202,
          json: { turn_id: "turn-1", status: "interrupting" },
        });
      },
    );
    await page.goto(`/w/${workspace.id}`);

    await page.getByRole("button", { name: "Stop agent" }).click();

    await expect(
      page.getByRole("button", { name: "Stopping agent" }),
    ).toBeDisabled();
    expect(interruptRequests).toBe(1);
  });
});
