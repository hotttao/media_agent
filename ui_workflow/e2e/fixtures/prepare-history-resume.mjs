import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { loadEnvFile } from "node:process";
import { createHash } from "node:crypto";

// Operator-only API fixture provisioning. Never print response bodies, tokens,
// prompts or signed URLs. Browser tests never receive the migration token.
loadEnvFile(resolve("e2e/.env"));
const token = process.env.E2E_API_TOKEN;
if (!token) throw new Error("User test API Token required");
const base = "http://192.168.2.41:18080";
const path = resolve("../media_worker/.fixture-ui-resume-ledger.json");
const configPath = resolve("../media_worker/.fixture-ui-resume-results.json");
const stage = process.argv[2];
async function request(method, route, body, migration = false) {
  const r = await fetch(
    `${migration ? "http://127.0.0.1:8002" : base}${route}`,
    {
      method,
      signal: AbortSignal.timeout(120000),
      headers: {
        "Content-Type": "application/json",
        ...(migration
          ? {
              "X-Media-Workflow-Migration-Token":
                process.env.MEDIA_WORKFLOW_MIGRATION_TOKEN,
            }
          : { Authorization: `Bearer ${token}` }),
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    },
  );
  if (!r.ok)
    throw new Error(
      `Fixture ${method} ${route.split("?")[0]} status=${r.status}; private response suppressed`,
    );
  return r.json();
}
function save(l, exclusive = false) {
  writeFileSync(path, JSON.stringify(l), {
    flag: exclusive ? "wx" : "w",
    mode: 0o600,
  });
}
const prefix = (d) =>
  d.step_runs
    .filter((s) => s.origin === "legacy_import")
    .map((s) => ({
      key: s.step_key,
      status: s.status,
      asset: s.output_asset_id,
      source: s.source_record_id,
    }));
const digest = (value) =>
  createHash("sha256").update(JSON.stringify(value)).digest("hex");

if (stage === "prepare") {
  if (existsSync(path))
    throw new Error(
      "Existing isolated fixture: inspect ledger, do not create again",
    );
  const me = await request("GET", "/api/auth/me");
  const originals = [];
  for (let page = 1; ; page++) {
    const d = await request(
      "GET",
      `/api/v1/jobs?status=awaiting_resume&page_size=200&page=${page}`,
    );
    originals.push(...d.items);
    if (originals.length >= d.total) break;
    if (!d.items.length) throw new Error("History audit incomplete");
  }
  const chosen = originals.find((j) =>
    j.source_record_id?.startsWith("frame:"),
  );
  if (!chosen) throw new Error("No three-image historical lineage available");
  const source = await request("GET", `/api/v1/jobs/${chosen.id}`);
  if (
    source.job.origin !== "legacy_import" ||
    source.job.status !== 7 ||
    source.job.submitted_by !== me.id ||
    prefix(source).length !== 3
  )
    throw new Error("Expected owned three-image historical prefix");
  const sourceWorkspace = await request(
    "GET",
    `/api/v1/workspaces/${source.job.workspace_id}`,
  );
  const frozen = await request(
    "GET",
    `/api/v1/workflows/${source.job.workflow_id}/versions/${source.job.workflow_version}`,
  );
  const scope = source.job.scope;
  if (frozen.steps.length !== 4)
    throw new Error("Four-step frozen source required");
  const l = {
    name: `ui-resume-mock-${Date.now()}`,
    source: source.job.id,
    organization: source.job.organization_id,
    owner: me.id,
    originals: originals.map((j) => j.id).sort(),
    prefixHash: digest(prefix(source)),
    templates: {},
  };
  save(l, true);
  const workspace = await request("POST", "/api/v1/workspaces", {
    name: l.name,
    organization_id: l.organization,
    team_id: sourceWorkspace.team_id,
    agent_type: sourceWorkspace.agent_type,
  });
  l.workspace = workspace.id;
  if (!l.workspace || workspace.organization_id !== l.organization)
    throw new Error("Isolated Workspace receipt invalid");
  save(l);
  for (const step of frozen.steps) {
    const tpl = await request(
      "GET",
      `/api/v1/templates/${step.template_id}/versions/${step.template_version}`,
    );
    const created = await request("POST", `/api/v1/templates?scope=${scope}`, {
      ...tpl,
      organization_id: l.organization,
      name: `${l.name}-${step.key}`,
      candidate_count: 1,
    });
    l.templates[step.key] = created.id;
    if (!created.id) throw new Error("Template create receipt invalid");
    save(l);
    await request("POST", `/api/v1/templates/${created.id}/publish`);
  }
  const workflow = await request("POST", `/api/v1/workflows?scope=${scope}`, {
    name: l.name,
    organization_id: l.organization,
    workspace_id: l.workspace,
    output_step_key: "video",
    steps: frozen.steps.map((s) => ({
      ...s,
      template_id: l.templates[s.key],
      template_version: 1,
      approval_policy: "always",
    })),
  });
  l.workflow = workflow.id;
  if (!l.workflow) throw new Error("Workflow create receipt invalid");
  save(l);
  await request("POST", `/api/v1/workflows/${l.workflow}/publish`);
  console.log(
    `Isolated Workspace and definition prepared workflow_id=${l.workflow}`,
  );
} else if (stage === "import") {
  if (!process.env.MEDIA_WORKFLOW_MIGRATION_TOKEN)
    throw new Error("Ephemeral migration token required");
  const l = JSON.parse(readFileSync(path));
  if (l.importStarted)
    throw new Error(
      "Prior import intent: inspect original Workspace lineage before replay",
    );
  const source = await request("GET", `/api/v1/jobs/${l.source}`);
  if (source.job.status !== 7 || digest(prefix(source)) !== l.prefixHash)
    throw new Error("Original history changed");
  const definition = await request("GET", `/api/v1/workflows/${l.workflow}`);
  if (definition.publication_status !== "registered")
    throw new Error("Isolated definition not yet registered");
  l.importStarted = true;
  save(l);
  const receipt = await request(
    "POST",
    "/api/internal/v1/jobs/import-incomplete",
    {
      source_group_id: source.job.source_record_id,
      organization_id: l.organization,
      workspace_id: l.workspace,
      owner_user_id: l.owner,
      workflow_id: l.workflow,
      workflow_version: 1,
      input: {
        product: source.job.input.product,
        virtual_ip: source.job.input.virtual_ip,
      },
      completed: prefix(source).map((p) => ({
        step_key: p.key,
        source_record_id: p.source,
        asset_id: p.asset,
      })),
    },
    true,
  );
  if (!receipt.job_id || receipt.status !== 7 || receipt.job_id === l.source)
    throw new Error("Import receipt invalid");
  l.job = receipt.job_id;
  save(l);
  const tpl = await request(
    "GET",
    `/api/v1/templates/${l.templates.video}/versions/1`,
  );
  const asset = await request(
    "GET",
    `/api/v1/generated-assets/gen_5db3b9bd-8450-47d9-a028-93c76398ffbc?organization_id=${l.organization}`,
  );
  const object = await request(
    "GET",
    `/api/v1/library-objects:resolve?organization_id=${l.organization}&object_path=${encodeURIComponent(asset.object_path)}`,
  );
  if (
    !object.content_type?.startsWith("video/") ||
    !object.url ||
    !asset.object_path.startsWith(`materials/${l.organization}/`)
  )
    throw new Error("Video fixture invalid");
  writeFileSync(
    configPath,
    JSON.stringify({
      tasks: [
        {
          job_id: l.job,
          step_key: "video",
          capability_key: tpl.capability_key,
          capability_version: tpl.capability_version,
          result_urls: [object.url],
        },
      ],
    }),
    { flag: "wx", mode: 0o600 },
  );
  console.log(
    `Isolated history imported job_id=${l.job}; private video fixture prepared`,
  );
} else throw new Error("Explicit prepare or import stage required");
