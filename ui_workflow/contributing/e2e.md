# E2E Testing Guide

## Running Tests

```bash
# Start the dev server first
yarn dev

# Run all E2E tests
E2E_USER_EMAIL=<email> E2E_USER_PASSWORD=<password> yarn test:e2e

# Local development mode (requires media services with MW_DEV_USER_ID=dev-user)
E2E_DEV_AUTH=true yarn test:e2e

# If Playwright's bundled browser is unavailable, use an installed Chrome
E2E_DEV_AUTH=true E2E_CHROMIUM_EXECUTABLE=/usr/bin/google-chrome yarn test:e2e

# Run a specific test
E2E_USER_EMAIL=<email> E2E_USER_PASSWORD=<password> yarn test:e2e --grep "roles"

# Slow shared workstation: explicitly extend diagnostics, not the normal budget
E2E_ACTION_TIMEOUT_MS=30000 yarn test:e2e --timeout=90000
# Explicitly allow a slower post-login application bootstrap on a loaded host
E2E_APP_READY_TIMEOUT_MS=90000 yarn test:e2e --timeout=180000

# Open the Playwright UI for debugging
E2E_USER_EMAIL=<email> E2E_USER_PASSWORD=<password> yarn test:e2e:ui
```

Standalone 页面测试可使用 `openStandaloneApp(page)`（`e2e/fixtures/base.ts`）：先打开页面，再等认证后的 Shell 标题出现，随后验证 App 交互。它复用上述启动预算（默认15秒），不延长业务断言、不绕过认证；启动失败与节点/表单交互失败应分别记录。

仅基础设施诊断可显式设置 `E2E_RETRY_NETWORK_BOOTSTRAP=true`：只有已观察到 `net::ERR_NETWORK_CHANGED` 且 Shell 启动失败时，在 App 操作开始前最多刷新文档一次，并记录该事件。默认关闭，不重跑测试、不重放业务POST、不改变生产页面加载逻辑。其他错误仍直接失败。

## Project Structure

```
e2e/
├── auth/
│   └── auth.setup.ts          # Login once, save session to storage-state.json
├── fixtures/
│   └── base.ts                # Playwright fixtures (register ResourcePage instances)
├── helpers/
│   ├── resource-page.ts       # Page object: navigation, show page actions
│   ├── table-helper.ts        # Table operations: CRUD, sort, column toggle, etc.
│   ├── form-helper.ts         # Form operations: fillInput, selectOption, submit, etc.
│   ├── yaml-import.ts         # YamlImportHelper for seeding test data via YAML import UI
│   ├── api-helper.ts          # Direct API calls (user/role/policy CRUD) via admin session
│   └── test-user-context.ts   # Multi-user helpers: loginAs, logout, TestUserContext
└── tests/
    └── *.spec.ts              # Test specs
```

## Test Organization

Each `test()` is tagged with one or more TestRail case IDs. Group tests by section using `test.describe()`:

```ts
import { test, expect } from "../fixtures/base";

test.describe("roles list", () => {
  test("list page shows expected columns", {
    tag: "@C2611652",
  }, async ({ roles }) => {
    await roles.goToList();
    await roles.table.waitForLoaded();
    const headers = roles.table.root.locator("thead th");
    await expect(headers.filter({ hasText: /name/i })).toBeVisible();
  });
});
```

- **Tag format**: `tag: "@C{case_id}"`
- **Multiple tags**: `tag: ["@C2611697", "@C2611664"]`
- **Describe groups**: list / detail / create / edit / delete

### Test data isolation

Each test that creates data must clean up after itself. Use `{ noWait: true }` for cleanup to avoid waiting for the delete to complete:

```ts
test("admin user can create a role with permissions", {
  tag: "@C2611697",
}, async ({ roles }) => {
  const uniqueName = `test-role-${Date.now()}`;

  await createRole(roles, uniqueName, ["Workspaces:Read"]);
  await roles.goToList();
  await roles.table.expectRowWithText(uniqueName);

  // Cleanup — noWait since we don't need to verify deletion
  await roles.table.deleteRow(uniqueName, { noWait: true });
});
```

For tests that actually verify delete behavior, omit `noWait` to wait for row removal:

```ts
test("can delete role from list", {
  tag: "@C2611721",
}, async ({ roles }) => {
  const uniqueName = `test-role-${Date.now()}`;
  await createRole(roles, uniqueName, ["Workspaces:Read"]);
  await roles.goToList();

  await roles.table.deleteRow(uniqueName);
  await roles.table.expectNoRowWithText(uniqueName);
});
```

Use `` `test-xxx-${Date.now()}` `` for unique names.

## Adding Tests for a New Resource

### Step 1: Register a fixture (`e2e/fixtures/base.ts`)

```ts
clusters: async ({ page }, use) => {
  await use(
    new ResourcePage(page, {
      routeName: "clusters",
      workspaced: true,        // set true if the route is under a workspace
      workspace: "default",    // defaults to "default"
    }),
  );
},
```

### Step 2: Write the spec (`e2e/tests/<resource>.spec.ts`)

See [Test Organization](#test-organization) above.

## Helper API

### ResourcePage

| Method | Description |
|---|---|
| `goToList()` | Navigate to list page, wait for table loaded |
| `goToCreate()` | Navigate to create page, wait for form visible |
| `goToEdit(id)` | Navigate to edit page, wait for form visible |
| `goToShow(id)` | Navigate to show page, wait for `show-page` visible |
| `clickCreate()` | Click the Create button on the list page |
| `showPageEdit()` | Show page → open actions menu → click Edit |
| `showPageDelete(name)` | Show page → open actions menu → click Delete → confirm → wait for row removal |

### TableHelper (`resource.table`)

| Method | Description |
|---|---|
| `waitForLoaded()` | Wait for table data to finish loading |
| `rows()` | Locator for all data rows |
| `rowWithText(text)` | Locator for a row containing the text |
| `expectRowWithText(text)` | Assert a row with the text exists |
| `expectNoRowWithText(text, options?)` | Assert no row with the text exists |
| `clickRowLink(text)` | Click the first link in a row |
| `editRow(text)` | Row actions → Edit |
| `deleteRow(text, options?)` | Row actions → Delete → confirm → wait for row removal. Pass `{ noWait: true }` to skip waiting (for cleanup-only scenarios) |
| `hasRowActions(text)` | Whether the row has an actions button (returns boolean) |
| `headerCell(text)` | Locator for a column header (string or RegExp) |
| `sort(columnText)` | Click sort trigger, verify sorting is active |
| `toggleColumn(columnName)` | Toggle column visibility via Columns dropdown |

### FormHelper (`resource.form`)

| Method | Description |
|---|---|
| `field(name)` | Locator for `[data-testid="field-{name}"]` |
| `fillInput(name, value)` | Clear and type into a text input |
| `fillTextarea(name, value)` | Clear and type into a textarea |
| `selectOption(name, optionText)` | Open a Radix Select and pick an option |
| `selectComboboxOption(name, optionText)` | Open a cmdk Combobox and pick an option (popover-safe) |
| `toggleCheckbox(name)` | Toggle a checkbox |
| `submit()` | Click the submit button |
| `cancel()` | Click the cancel button |

The `name` parameter matches the react-hook-form field path (e.g. `"metadata.name"`, `"spec.type"`).

### ApiHelper (`apiHelper` fixture)

Makes direct API calls using the admin page's auth session. Useful for fast test data setup/cleanup without going through the UI.

| Method | Description |
|---|---|
| `createUser(name, email, password)` | Create a user via admin API, returns `user_profile` ID |
| `deleteUser(name)` | Soft-delete a user_profile by name |
| `getUserId(name)` | Get user_profile ID by name |
| `createRole(name, permissions)` | Create a role with the given permissions |
| `deleteRole(name)` | Soft-delete a role by name |
| `createPolicy(name, userId, roleName, global?)` | Create a role assignment |
| `deletePolicy(name)` | Soft-delete a role assignment by name |
| `softDelete(resource, name)` | Generic soft-delete for any resource |

```ts
// Fast setup: create user + role + policy via API instead of UI forms
const userId = await apiHelper.createUser(userName, email, "Test@123456");
await apiHelper.createRole(roleName, ["workspace:read"]);
await apiHelper.createPolicy(policyName, userId, roleName, true);

// Cleanup in finally block (reverse dependency order)
await apiHelper.deletePolicy(policyName).catch(() => {});
await apiHelper.deleteRole(roleName).catch(() => {});
await apiHelper.deleteUser(userName).catch(() => {});
```

### Multi-user helpers (`loginAs`, `logout`, `createTestUser`)

For tests that need to log in as a different user (permission tests, login verification, password change).

Import `loginAs` and `logout` from `../helpers/test-user-context`:

```ts
import { loginAs, logout } from "../helpers/test-user-context";
```

**`loginAs(browser, apiHelper, email, password)`** — Create a clean browser context (no inherited admin session), navigate to login, authenticate, and return `{ page, context }`. Caller must close the context when done.

```ts
const { page, context } = await loginAs(browser, apiHelper, email, password);
await expect(page).toHaveURL(/\/#\/dashboard/);
// ... test assertions on `page` ...
await context.close();
```

**`logout(page)`** — Click user dropdown → Logout, wait for login page to appear.

```ts
await logout(page);
// Now on the login page — can fill credentials and login again
await page.locator('input[name="email"]').fill(email);
await page.locator('input[name="password"]').fill(password);
await page.getByRole("button", { name: /sign in/i }).click();
```

**`createTestUser` fixture** — Factory function that creates a user with specific permissions, logs in, and auto-cleans up after the test. Best for permission tests.

```ts
test("user without permission sees no create button", async ({ createTestUser, browser }) => {
  const testUser = await createTestUser(["role:read"]);
  // testUser.page is logged in with only role:read permission
  await testUser.page.goto("/#/roles");
  await expect(testUser.page.getByRole("link", { name: /create/i })).toBeHidden();
  // No manual cleanup needed — handled automatically after test
});
```

## `data-testid` Reference

| Attribute | Location |
|---|---|
| `form` | Form wrapper |
| `field-{name}` | Form field container |
| `form-submit` | Submit button |
| `form-cancel` | Cancel button |
| `table` | Table wrapper |
| `table-body` | Table body |
| `table-loading` | Loading row |
| `table-empty` | Empty state row |
| `row-actions-trigger` | Row action menu button |
| `show-page` | Show page container |
| `show-actions-trigger` | Show page action menu button |
| `sort-trigger` | Column header sort button (also exposes `data-sort-direction`) |
| `global-roles-card` | User show page: Global Roles card |

When adding new testable UI elements, prefer `data-testid` over CSS class selectors.

## Locator Strategy

Prefer semantic locators over positional or CSS-based ones. Priority (highest → lowest):

1. **`getByRole()`** — buttons, links, menu items, dialogs, options, etc. Most resilient to UI changes.
2. **`getByText()` / `getByLabel()`** — visible text or label associations. Use `{ exact: true }` to avoid partial match ambiguity.
3. **`data-testid`** — structural wrappers that have no accessible role (pages, forms, triggers, table bodies).
4. **CSS selectors** — only when the above are insufficient (e.g., `button[role="combobox"]` for Radix Select disabled check).

**Avoid:**
- **Positional indexing** like `.nth(2)`, `.locator("td").nth(3)` — breaks when column order changes. Instead, assert on text content within the row or use a semantic locator.
- **CSS class selectors** — classes are implementation details and change frequently. Use `data-testid` instead.

**When no suitable locator exists**, add a `data-testid` attribute to the source component rather than falling back to fragile selectors. A targeted `data-testid` is better than a positional or CSS-class workaround.

## Notes

- **Delete cleanup optimization**: `deleteRow()` waits up to 30s for the row to disappear by default. For cleanup-only scenarios (not testing delete), use `deleteRow(name, { noWait: true })` to skip waiting — this saves 10-20s per call. Only tests that verify delete behavior should wait for row removal.
- **Cancel navigation**: The cancel button uses `history.back()`. Always navigate to the list page before clicking Create, so that cancel returns to a valid page.
- **API error logging**: The `page` fixture in `base.ts` logs all 4xx/5xx responses to stdout.
- **Do not change container networks during browser tests**: finish Compose restarts and testcontainers tests before Playwright. Creating/removing Docker network interfaces can cause Chromium `ERR_NETWORK_CHANGED`, interrupting Vite module downloads and leaving a blank page unrelated to the UI assertions.

## Pitfalls

### `selectOption` vs `selectComboboxOption`

Use **`selectOption`** for Radix `<Select>` fields — options render inside a simple dropdown.

Use **`selectComboboxOption`** for cmdk `<Combobox>` fields — the component uses a Radix Popover + cmdk Command internally. Radix Popover keeps closed popover content mounted in the DOM (with `data-state="closed"`). If two comboboxes on the same form share option names (e.g., user "admin" and role "admin"), `selectOption` will hit a strict mode violation because `getByRole("option")` matches options from both popovers. `selectComboboxOption` scopes the click to `[data-state="open"][role="dialog"]` to target only the active popover.

### Radix Select disabled check

Radix Select renders both a `<button role="combobox">` trigger and a hidden `<select>` element. Using `getByRole("combobox")` may match both and cause a strict mode violation. To check if a Select is disabled, use the CSS selector instead:

```ts
await expect(
  scopeField.locator('button[role="combobox"]'),
).toBeDisabled();
```

### `browser.newContext()` inherits admin session

The Playwright config sets `storageState` for the chromium project, so **every** `browser.newContext()` inherits the admin's auth session. To create a truly unauthenticated context (e.g., for `loginAs`), you must explicitly clear it:

```ts
const context = await browser.newContext({
  baseURL,
  storageState: { cookies: [], origins: [] },
});
```

The `loginAs` helper already handles this — prefer using it over manual context creation.

### Edit form race condition

After navigating to an edit page, the form renders before the API data populates the inputs. If you fill an input immediately, the fetched data will overwrite your value. Wait for `form-submit` to be enabled (which indicates data has loaded) before modifying fields:

```ts
await users.table.editRow(name);
// ✅ Wait for data to load
await expect(page.locator('[data-testid="form-submit"]')).toBeEnabled();
await users.form.fillInput("spec.email", newEmail);

// ❌ Don't do this — value may be overwritten by API fetch
await users.table.editRow(name);
await users.form.fillInput("spec.email", newEmail);
```

### Test data naming

Avoid keywords like "create", "edit", "delete" in test data names (e.g., `test-wp-create-...`). These can collide with UI button text in selectors like `getByRole("link", { name: /create/i })`. Use neutral prefixes: `test-wp-new-`, `test-wp-del-`, etc.

## 单条 Workflow 真实执行门禁

`E2E_SINGLE_WORKFLOW_STAGE=submit` 运行 `e2e/tests/workflow-single-execution.spec.ts --grep 'submit one'`：审计全部当前User任务，在现有授权Workspace创建隔离定义，UI选择输入并只提交一次。独立私有账本位于 `media_worker/.fixture-ui-single-ledger.json`（0600，已忽略），避免 Playwright 清理 test-results 后丢失原 Job。账本已记 submitStarted 时不得重新提交；未知响应只读查询原意图/隔离定义，不删除账本绕过保护。

随后按 `TestProviderCapabilityFixtureConfig` 为该已确认 Job 准备私有结果配置，在测试进程外启动客户端 provider_fixture，再以 `E2E_SINGLE_WORKFLOW_STAGE=review` 运行同文件 `--grep 'review the original' --timeout=600000`。UI验证两候选审批、反馈再生成、always策略单候选审批和四产物；只归档本次模板/Workflow，保留成功 Job 与产物。完成后停止测试客户端，不关业务服务。不启动历史待继续队列，不在E2E内fake Worker或调用真实厂商。

### 独立部分恢复样本

`E2E_WORKFLOW_RECOVERY=true` 只将 submit 门禁的隔离样本账本切换到 `.fixture-ui-recovery-ledger.json`，不允许复用成功样本做重跑；此模式禁用完整 review 门禁。先准备该 Job 的私有 Provider 配置，在测试外启动 media_worker，设置 `MEDIA_WORKER_FIXTURE_FAIL_STEP=model_image` 和 `MEDIA_WORKER_FIXTURE_FAIL_CALL=1`。确认首节点永久失败后，运行 `workflow-recovery.spec.ts --grep 'continue an isolated'`，显式设置 `E2E_WORKFLOW_RECOVERY_STAGE=retry`：UI失败继续，首节点两候选后UI取消，禁止下游完整执行。

重跑必须使用另一条明确 failed 的隔离样本，不能复用上面已 cancelled 的 Job。以 `E2E_WORKFLOW_RECOVERY=rerun` 使用单独 `.fixture-ui-rerun-ledger.json` 创建部分样本，配置独立 Provider mock 首节点失败一次后停止客户端；再以 `E2E_WORKFLOW_RECOVERY_STAGE=rerun` 运行同文件的 rerun 门禁：按原冻结版本选择至少一个不同的真实资源，提交原 Job 的 input_override，再UI取消，只归档本次模板/定义。命令发起前记录账本；响应不确定时只读核对，不重置账本重发。未经过此专用门禁不得 Resume 原历史记录，不自动启动119个历史 Job。候选数据加载使用显式60秒等待，与其他真实表单门禁一致，不延长生产代码预算。

### 历史补缺的隔离门禁

`prepare-history-resume.mjs prepare` 经用户API建立同企业新测试Workspace及冻结定义副本，保留原119条历史ID与前缀摘要，不改原记录。仅在用户明确同意后临时启用受Token保护导入入口，以进程环境中的临时Token执行 `import`，在EXIT清理中恢复禁用；不用日志或浏览器传递迁移Token。私有账本 `.fixture-ui-resume-ledger.json` 为0600，配置/Token均不提交。

谱系冻结的旧能力使用显式 `media_ai_fixture` 兼容profile，独立客户端仅注册 `media.video.generate@1`，已完成图片没有对应客户端能力。新Provider流程另按前面的Provider门禁验收，不把旧能力混入新公开目录。`E2E_HISTORY_RESUME=true` 运行 `workflow-history-resume.spec.ts`：只补视频使用的movement，Resume一次，单候选明确选择与确认，视频完成后逐条检查原119任务不变；保留结果，仅归档副本定义/模板。已接受Resume在私有账本内记录，后续仅观察/审批原Job，不再提交Resume。测试结束停止该mock，保留业务服务和标记清楚的测试Workspace。

`E2E_HISTORY_MEDIA=true` 运行只读 `workflow-history-media.spec.ts`，检查完成样本的候选新签名HTTP206及真实videoWidth，不以HTTP可读代替浏览器可预览。未确认审批不得重发新决定；单个候选同样先选radio再确认，不通过改审批策略取得绿色测试。
