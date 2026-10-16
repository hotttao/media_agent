import { expect, type Page, test } from "@playwright/test";
import type {
  CatalogResource,
  MCPDefinition,
} from "../../src/domains/agent-catalog/types";
import type { Binding } from "../../src/domains/agent-configuration/types";

async function fixture(page: Page) {
  let prompt: CatalogResource | undefined;
  let skill: CatalogResource | undefined;
  let file = "";
  let published = "";
  let binding: Binding | undefined;
  let definition: MCPDefinition = {
    definition_id: "search",
    organization_id: "org",
    service_name: "Search",
    endpoint: "https://mcp.example.test/mcp",
    transport: "streamable_http",
    authentication_type: "api_key",
    status: "enabled",
    version: 1,
  };
  const promptHistory = new Map<number, CatalogResource>();
  const requests: {
    path: string;
    method: string;
    body: Record<string, unknown>;
  }[] = [];
  let configurationVersion = 4;
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    const body = (request.postData() ? request.postDataJSON() : {}) as Record<
      string,
      unknown
    >;
    if (method !== "GET") requests.push({ path, method, body });
    const json = (value: unknown, status = 200) =>
      route.fulfill({ json: value, status });
    if (path === "/api/auth/me")
      return json({
        id: "owner",
        username: "designer",
        role: "admin",
        auth_source: "password",
      });
    if (path === "/api/organizations")
      return json({
        organizations: [
          { id: "org", name: "Studio", status: "active", my_role: "admin" },
        ],
      });
    if (path === "/api/teams")
      return json({
        teams: [
          {
            id: "team",
            name: "Creative",
            organization_id: "org",
            status: "active",
            my_role: "member",
          },
        ],
      });
    if (path === "/api/v1/prompts") {
      if (method === "POST") {
        prompt = {
          id: "prompt",
          name: String(body.name),
          organization_id: "org",
          owner_user_id: "owner",
          visibility: "private",
          my_permission: "owner",
          version: 1,
          content: String(body.content ?? ""),
        };
        promptHistory.set(1, { ...prompt });
        return json(prompt, 201);
      }
      return json({ prompts: prompt ? [prompt] : [] });
    }
    if (path === "/api/v1/prompts/prompt/versions")
      return json({
        versions: [...promptHistory.values()].map((value) => ({
          version: value.version,
          name: value.name,
          content: value.content,
        })),
      });
    if (path.match(/^\/api\/v1\/prompts\/prompt\/versions\/\d+$/))
      return json(promptHistory.get(Number(path.split("/").pop())));
    if (path === "/api/v1/prompts/prompt/access" && prompt) {
      prompt = {
        ...prompt,
        version: prompt.version + 1,
        visibility: body.visibility as CatalogResource["visibility"],
        grants: body.grants as CatalogResource["grants"],
      };
      promptHistory.set(prompt.version, { ...prompt });
      return json(prompt);
    }
    if (path === "/api/v1/prompts/prompt/rollback" && prompt) {
      const old = promptHistory.get(Number(body.target_version));
      prompt = {
        ...prompt,
        version: prompt.version + 1,
        content: old?.content,
        name: old?.name ?? prompt.name,
      };
      promptHistory.set(prompt.version, { ...prompt });
      return json(prompt);
    }
    if (path === "/api/v1/prompts/prompt" && prompt) {
      if (method === "PUT") {
        prompt = {
          ...prompt,
          name: String(body.name),
          content: String(body.content),
          version: prompt.version + 1,
        };
        promptHistory.set(prompt.version, { ...prompt });
      }
      return json(prompt);
    }
    if (path === "/api/v1/skills") {
      if (method === "POST") {
        skill = {
          id: "skill",
          name: String(body.name),
          description: String(body.description),
          organization_id: "org",
          owner_user_id: "owner",
          visibility: "private",
          my_permission: "owner",
          status: "active",
          version: 1,
          published_version: 0,
          has_draft: true,
          source_kind: "native",
        };
        file = `---\nname: ${body.slug}\ndescription: ${body.description}\n---\n\n`;
        return json(skill, 201);
      }
      return json({ skills: skill ? [skill] : [] });
    }
    if (path === "/api/v1/skills/skill/files")
      return json({
        files: [{ path: "SKILL.md", size: file.length }],
        expected_version: skill?.version,
      });
    if (path === "/api/v1/skills/skill/file" && skill) {
      if (method === "PUT") {
        file = Buffer.from(String(body.content), "base64").toString();
        skill = { ...skill, version: skill.version + 1, has_draft: true };
        return json(skill);
      }
      const version = Number(
        new URL(request.url()).searchParams.get("version"),
      );
      return json({
        path: "SKILL.md",
        content: Buffer.from(version ? published : file).toString("base64"),
        binary: false,
        executable: false,
        expected_version: skill.version,
      });
    }
    if (path === "/api/v1/skills/skill/publish" && skill) {
      skill = {
        ...skill,
        version: skill.version + 1,
        published_version: 1,
        has_draft: false,
      };
      published = file;
      return json(skill);
    }
    if (path === "/api/v1/skills/skill/versions")
      return json({
        versions: skill?.published_version
          ? [
              {
                version: 1,
                slug: "video-writing",
                description: "Write video scripts",
              },
            ]
          : [],
      });
    if (path === "/api/v1/skills/skill/versions/1/package")
      return json({
        content: Buffer.from(
          "browser download fixture; real ZIP verified by backend integration",
        ).toString("base64"),
        filename: "video-writing-v1.zip",
      });
    if (path === "/api/v1/skills/skill") return json(skill);
    if (path === "/api/v1/mcp-definitions") {
      if (method === "POST")
        definition = { ...definition, ...body, version: 1 } as MCPDefinition;
      return json(
        method === "POST" ? definition : { definitions: [definition] },
        method === "POST" ? 201 : 200,
      );
    }
    if (path === "/api/v1/mcp-definitions/search") return json(definition);
    if (path === "/api/v1/workspaces")
      return json({
        workspaces: [
          {
            id: "workspace",
            name: "Creative workspace",
            organization_id: "org",
            team_id: "team",
            agent_configuration_id: "config",
            agent_id: "codex",
            agent_type: "codex",
            api_key_configured: true,
            configuration_version: configurationVersion,
          },
        ],
      });
    if (path === "/api/v1/workspaces/workspace/permissions")
      return json({ can_view: true, can_manage: true });
    if (path === "/api/v1/workspaces/workspace/mcp-bindings") {
      if (method === "POST") {
        binding = {
          binding_id: "binding",
          workspace_id: "workspace",
          definition_id: String(body.definition_id),
          definition_version: Number(body.definition_version),
          enabled: true,
          credential_configured: true,
          credential_header: "X-API-Key",
          version: 1,
        };
        return json(binding, 201);
      }
      return json({ bindings: binding ? [binding] : [] });
    }
    if (path === "/api/v1/workspaces/workspace/mcp-bindings/binding") {
      if (method === "PUT" && binding)
        binding = { ...binding, version: binding.version + 1 };
      return json(binding);
    }
    if (path === "/api/v1/workspaces/workspace/mcp-bindings/binding/versions")
      return json({ bindings: binding ? [binding] : [] });
    if (path === "/api/v1/workspaces/workspace/agent-metadata")
      return configurationVersion === 4
        ? json({ error: "metadata_not_found" }, 404)
        : json({
            agent_config_id: "config",
            version: configurationVersion,
            prompt: { mode: "custom", content: "Be precise" },
            resources: binding
              ? [
                  {
                    kind: "media_plugin.mcp_binding",
                    id: "binding",
                    version: 1,
                  },
                ]
              : [],
          });
    if (path === "/api/v1/workspaces/workspace/agent-metadata:confirm") {
      configurationVersion++;
      return json({ context_version: configurationVersion });
    }
    if (path === "/api/chat/sessions")
      return json({
        sessions: [
          {
            id: "session",
            workspace_id: "workspace",
            agent_id: "codex",
            agent_type: "codex",
            title: "Pinned session",
            status: "active",
            chat_status: "human",
            context_version: 4,
          },
        ],
      });
    if (path.endsWith("/messages")) return json({ messages: [] });
    if (path.endsWith("/requests")) return json({ requests: [] });
    if (path.endsWith("/events"))
      return route.fulfill({
        contentType: "text/event-stream",
        body: "event: ready\ndata: {}\n\n",
      });
    if (path.endsWith("/turns"))
      return json({ turn_id: "turn", status: "running" });
    return json({ error: "unmocked_fixture_route" }, 404);
  });
  return { requests };
}
test("prompt creation, editing, immutable history, sharing and narrow layout", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/library/org");
  await page.getByRole("button", { name: "New prompt", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Brand voice");
  await dialog.getByLabel("Prompt content").fill("Original instructions");
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Brand voice" }),
  ).toBeVisible();
  await page.getByLabel("Prompt content").fill("Revised instructions");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByLabel("Version history")).toContainText(
    "Current · v2",
  );
  await page.getByLabel("Version history").selectOption("1");
  await expect(page.getByLabel("Prompt content")).toHaveValue(
    "Original instructions",
  );
  await expect(page.getByLabel("Prompt content")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await page.getByRole("button", { name: "Restore this version" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(page.getByLabel("Version history")).toHaveValue("0");
  await page.getByRole("button", { name: "Sharing", exact: true }).click();
  await page.getByLabel("Visibility").selectOption("team");
  await page.getByLabel("Add department").selectOption("team");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(
    state.requests.filter((item) =>
      item.path.endsWith("agent-metadata:confirm"),
    ),
  ).toHaveLength(0);
  expect(
    state.requests.find((item) => item.path.endsWith("/rollback"))?.body
      .target_version,
  ).toBe(1);
  await page.screenshot({ path: "/tmp/agent-product-prompt-desktop.png" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await page.getByRole("button", { name: /Brand voice/ }).click();
  await expect(page.getByLabel("Prompt content")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({ path: "/tmp/agent-product-prompt-mobile.png" });
});
test("Skill file editing, explicit publication, fixed history and download", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/library/org");
  await page.getByRole("button", { name: "Skills", exact: true }).click();
  await page.getByRole("button", { name: "New skill", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Video writing");
  await dialog
    .getByLabel("Description", { exact: true })
    .fill("Write video scripts");
  await dialog
    .getByLabel("Skill name (lowercase letters, digits, hyphens)")
    .fill("video-writing");
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByLabel("File content")).toContainText("description:");
  await page
    .getByLabel("File content")
    .fill(
      "---\nname: video-writing\ndescription: Write video scripts\n---\n\nUse a clear opening.\n",
    );
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await page
    .getByRole("button", { name: "Publish version", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(page.getByLabel("Version history")).toContainText(
    "Published · v1",
  );
  await page.getByLabel("Version history").selectOption("1");
  await expect(page.getByLabel("File content")).toHaveJSProperty(
    "readOnly",
    true,
  );
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download", exact: true }).click();
  expect((await download).suggestedFilename()).toBe("video-writing-v1.zip");
  expect(state.requests.some((item) => item.path.endsWith("/publish"))).toBe(
    true,
  );
  expect(
    state.requests.some((item) => item.path.endsWith("agent-metadata:confirm")),
  ).toBe(false);
  await page.screenshot({ path: "/tmp/agent-product-skill-desktop.png" });
});
test("Workspace MCP credential stays write-only and configuration confirmation preserves the old session", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/w/workspace");
  const settings = page.getByTestId("slot-slot-b");
  await expect(
    settings.getByText("Workspace MCP connections", { exact: true }),
  ).toBeVisible();
  await settings
    .getByRole("button", { name: "Add connection", exact: true })
    .click();
  await page
    .getByLabel("Credential", { exact: true })
    .fill("private-browser-fixture");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(settings.getByLabel("Search", { exact: true })).toBeVisible();
  await settings
    .getByRole("combobox", { name: "System prompt", exact: true })
    .selectOption("custom");
  await settings.getByLabel("Custom prompt content").fill("Be precise");
  await settings.getByLabel("Search", { exact: true }).check();
  await settings
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  expect(
    state.requests.some((item) => item.path.endsWith("agent-metadata:confirm")),
  ).toBe(false);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm and save", exact: true })
    .click();
  await expect(settings.getByRole("status")).toContainText(
    "Configuration saved",
  );
  await expect(
    page.getByRole("textbox", { name: "Message Codex", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Message Codex", exact: true })
    .fill("Use my pinned configuration");
  await page.getByRole("button", { name: "Send message" }).click();
  await expect
    .poll(
      () =>
        state.requests.filter((item) => item.path.endsWith("/turns")).length,
    )
    .toBe(1);
  const confirmed = state.requests.find((item) =>
    item.path.endsWith("agent-metadata:confirm"),
  );
  expect(confirmed?.body.prompt).toEqual({
    mode: "custom",
    content: "Be precise",
  });
  expect(JSON.stringify(confirmed?.body)).not.toContain(
    "private-browser-fixture",
  );
  expect(
    await page.evaluate(() => JSON.stringify({ ...localStorage })),
  ).not.toContain("private-browser-fixture");
  await settings.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(page.getByLabel("Credential", { exact: true })).toHaveValue("");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await page.screenshot({
    path: "/tmp/agent-product-configuration-desktop.png",
  });
});

test("first Agent startup prevents duplicate Session creation", async ({
  page,
}) => {
  const state = await fixture(page);
  let release: (() => void) | undefined;
  let started: (() => void) | undefined;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let created = false;
  const requested = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route(/\/api\/chat\/sessions(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "POST") {
      if (!created) return route.fallback();
      return route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          sessions: [
            {
              id: "second-session",
              workspace_id: "workspace",
              agent_id: "codex",
              agent_type: "codex",
              title: "Session 2",
              status: "active",
              context_version: 4,
            },
          ],
        }),
      });
    }
    started?.();
    await pending;
    created = true;
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        id: "second-session",
        workspace_id: "workspace",
        agent_id: "codex",
        agent_type: "codex",
        title: "Session 2",
        status: "active",
        context_version: 4,
      }),
    });
  });
  await page.goto("/w/workspace");
  const create = page.getByRole("button", { name: "New session", exact: true });
  await create.click();
  await requested;
  await expect(create).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Create session", exact: true }),
  ).toBeDisabled();
  release?.();
  await expect(create).toBeEnabled();
  await expect(page.getByTestId("session-list")).toContainText("Session 2");
  expect(state.requests.filter((r) => r.path.endsWith("/turns"))).toHaveLength(
    0,
  );
});

test("native permission HTTP payload renders options and resolves the chosen action", async ({
  page,
}) => {
  await fixture(page);
  let resolved = false;
  let response: unknown;
  const request = {
    request_id: "permission",
    turn_id: "turn",
    request_type: "permission",
    payload_json: JSON.stringify({
      tool_call: { title: "Read project instructions" },
      options: [
        { option_id: "allow_once", name: "Allow Once", kind: "allow_once" },
      ],
    }),
    status: "pending",
    created_at: 100,
  };
  await page.route("**/api/chat/sessions/session/requests", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        requests: [{ ...request, status: resolved ? "resolved" : "pending" }],
      }),
    }),
  );
  await page.route(
    "**/api/chat/sessions/session/turns/turn/requests/permission/resolve",
    async (route) => {
      response = JSON.parse(route.request().postDataJSON().response_json);
      resolved = true;
      return route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          ...request,
          status: "resolved",
          response_json: JSON.stringify(response),
          resolved_at: 200,
        }),
      });
    },
  );
  await page.goto("/w/workspace");
  const card = page.getByLabel("permission request", { exact: true });
  await expect(card).toContainText("Read project instructions");
  await card.getByRole("button", { name: "Allow Once", exact: true }).click();
  await expect(card).toHaveCount(0);
  expect(response).toEqual({ option_id: "allow_once" });
});

test("paused Session requires explicit resume before sending", async ({
  page,
}) => {
  const state = await fixture(page);
  let resumed = false;
  let resumeBody: Record<string, unknown> = {};
  await page.route(/\/api\/chat\/sessions(?:\?.*)?$/, (route) =>
    route.fulfill({
      json: {
        sessions: [
          {
            id: "session",
            workspace_id: "workspace",
            agent_id: "codex",
            agent_type: "codex",
            title: "Paused session",
            status: "active",
            chat_status: "human",
            context_version: 4,
            runtime_status: resumed ? "active" : "paused",
            execution_epoch: 3,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/sessions/session/resume", async (route) => {
    resumeBody = route.request().postDataJSON();
    resumed = true;
    return route.fulfill({
      json: { session_id: "session", status: "active", execution_epoch: 3 },
    });
  });
  await page.goto("/w/workspace");
  await expect(
    page.getByRole("button", { name: "Resume session", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Send message", exact: true }),
  ).toHaveCount(0);
  expect(state.requests.filter((r) => r.path.endsWith("/turns"))).toHaveLength(
    0,
  );
  await page
    .getByRole("button", { name: "Resume session", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Send message", exact: true }),
  ).toBeVisible();
  expect(resumeBody.expected_epoch).toBe(3);
  expect(typeof resumeBody.operation_id).toBe("string");
  expect(state.requests.filter((r) => r.path.endsWith("/turns"))).toHaveLength(
    0,
  );
});

for (const runtimeStatus of ["paused", "active"]) {
  test(`older ${runtimeStatus} session offers current configuration without upgrading or replaying it`, async ({
    page,
  }) => {
    await page.addInitScript(() =>
      localStorage.setItem("tos-language", "en-US"),
    );
    await fixture(page);
    const oldSession = {
      id: "old-session",
      workspace_id: "workspace",
      agent_id: "codex",
      agent_type: "codex",
      title: "Original session",
      status: "active",
      chat_status: "human",
      context_version: 1,
      runtime_status: runtimeStatus,
      execution_epoch: 2,
    };
    let newSession: typeof oldSession | undefined;
    const writes: string[] = [];
    await page.route(/\/api\/chat\/sessions(?:\?.*)?$/, async (route) => {
      if (route.request().method() === "POST") {
        writes.push(route.request().url());
        expect(route.request().postDataJSON()).toEqual({
          workspace_id: "workspace",
          title: "Session 2",
        });
        newSession = {
          ...oldSession,
          id: "new-session",
          title: "Session 2",
          context_version: 4,
          runtime_status: "active",
        };
        return route.fulfill({ json: newSession, status: 201 });
      }
      return route.fulfill({
        json: {
          sessions: newSession ? [newSession, oldSession] : [oldSession],
        },
      });
    });
    await page.route("**/api/v1/sessions/*/resume", (route) => {
      writes.push(route.request().url());
      return route.abort();
    });
    await page.route("**/api/chat/sessions/*/turns", (route) => {
      writes.push(route.request().url());
      return route.abort();
    });
    await page.goto("/w/workspace?session=old-session");
    const notice = page.getByTestId("session-configuration-notice");
    await expect(notice).toContainText(
      "configuration 1; the workspace now uses 4",
    );
    if (runtimeStatus === "active")
      await expect(
        page.getByRole("button", { name: "Send message", exact: true }),
      ).toBeVisible();
    else
      await expect(
        page.getByRole("button", { name: "Resume session", exact: true }),
      ).toBeVisible();
    expect(writes).toHaveLength(0);
    await notice
      .getByRole("button", {
        name: "New session with current configuration",
        exact: true,
      })
      .click();
    await expect(page).toHaveURL(/session=new-session/);
    await expect(notice).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Send message", exact: true }),
    ).toBeVisible();
    expect(writes).toHaveLength(1);
    await page
      .getByTestId("session-list")
      .getByRole("button", { name: /Original session/ })
      .click();
    await expect(notice).toContainText("configuration 1");
    expect(oldSession.context_version).toBe(1);
    expect(writes).toHaveLength(1);
  });
}
