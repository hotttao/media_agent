// One-shot maintenance. Default is inventory only; apply requires verified backups.
import { parseArgs } from "node:util";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join, isAbsolute } from "node:path";
import { spawnSync } from "node:child_process";
const { values } = parseArgs({ options: {
  "backup-dir": { type: "string" }, "keep-workflow": { type: "string" },
  "organization": { type: "string" }, "apply": { type: "boolean", default: false },
} });
const backup = values["backup-dir"], keep = values["keep-workflow"], org = values.organization;
if (!backup || !isAbsolute(backup) || !existsSync(join(backup, "SHA256SUMS")) ||
    !/^wf_[a-f0-9-]+$/.test(keep || "") || !/^[a-zA-Z0-9_-]+$/.test(org || ""))
  throw new Error("Explicit private backup directory, organization and retained workflow required");
function command(args, input) {
  const r = spawnSync(args[0], args.slice(1), { input, encoding: "utf8", maxBuffer: 32 << 20 });
  if (r.status !== 0) {
    writeFileSync(join(backup, "maintenance-error.log"), r.stderr || "command failed", { mode: 0o600 });
    throw new Error("Maintenance command failed; private diagnostics retained");
  }
  return r.stdout;
}
function sql(db, text) {
  return command(["docker", "exec", "-i", "docker-compose-postgres-1", "sh", "-c",
    'psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -At', "sh", db], text);
}
function rows(db, table, fields = "*") {
  return JSON.parse(sql(db, `SELECT COALESCE(json_agg(q),'[]'::json) FROM (SELECT ${fields} FROM ${table}) q;`));
}
function ids(items) {
  return items.map(x => {
    if (!/^[a-zA-Z0-9_-]+$/.test(x.id)) throw new Error("Invalid resource ID");
    return `'${x.id}'`;
  }).join(",") || "NULL";
}
const fixture = /^(e2e[_-]|nc_(wf|tpl|scene|vip)_|acl-(gateway|multi)-|publication-gate-|ui-(flow|product-route|single-mock|recovery-mock|resume-mock|contract|material)-|四节点.*联调)/;
// Explicit legacy migration definitions superseded by the retained Provider workflow.
const legacyWorkflows = new Set([
  "wf_c1aa6321-9394-46ed-bacf-121e9361c29a", "wf_4642faca-65d7-48d4-9f96-1fbc67c6f53e",
  "wf_ed878c49-f6a7-47f4-8866-03420ac5ffec", "wf_793446b8-c078-4735-8a18-3b40a579f2f5",
]);
const legacyTemplates = new Set([
  "tpl_ff3340b6-2793-45dc-a8fc-6106d3cd2545", "tpl_d6d7857c-66b2-4c99-8d44-8308e3aa5af3",
  "tpl_83897f2f-7738-4abb-9961-aba3fa07b935", "tpl_995bf505-f75f-4b70-9934-f327ef3e5684",
  "tpl_30d8ce90-56a3-4c31-a206-b8497ea83568", "tpl_dea9d422-82bc-4b27-ae52-e4ba9f3f4b7a",
  "tpl_9e03e522-41f0-4fe1-b2dd-d61a9f4f36eb",
]);
const workflows = rows("media_workflow", "media_workflow.workflows", "id,name,status");
const kept = workflows.find(w => w.id === keep);
if (!kept || kept.status !== "published") throw new Error("Retained published workflow missing");
const removedWorkflows = workflows.filter(w => w.id !== keep && (fixture.test(w.name) || legacyWorkflows.has(w.id)));
const workflowIDs = new Set(removedWorkflows.map(w => w.id));
const jobs = rows("media_workflow", "media_workflow.jobs", "id,workflow_id,origin,source_system,organization_id,conductor_workflow_id");
const removedJobs = jobs.filter(j =>
  (j.origin === "legacy_import" && j.source_system === "media_ai" && j.organization_id === org) ||
  (j.origin === "native" && (workflowIDs.has(j.workflow_id) || j.workflow_id === keep)));
const jobIDs = new Set(removedJobs.map(j => j.id));
const templates = rows("media_workflow", "media_workflow.templates", "id,name");
const steps = rows("media_workflow", "media_workflow.workflow_steps", "id,workflow_id,template_id");
const retainedTemplates = new Set(steps.filter(s => !workflowIDs.has(s.workflow_id)).map(s => s.template_id));
const removedTemplates = templates.filter(t => !retainedTemplates.has(t.id) && (fixture.test(t.name) || legacyTemplates.has(t.id)));
const pushes = rows("media_workflow", "media_workflow.historical_video_pushes", "id,organization_id,source_system")
  .filter(p => p.organization_id === org && p.source_system === "media_ai");
const assets = rows("media-materials", "media_materials.generated_assets", "id,origin,job_id,organization_id,source_system,object_path");
const removedAssets = assets.filter(a => (a.origin === "native" && jobIDs.has(a.job_id)) ||
  (a.origin === "legacy_import" && a.organization_id === org && a.source_system === "media_ai"));
const libraryFixture = r => fixture.test(r.name) || ["updated-material", "updated-product", "updated-vip", "Agent proxy temporary text fixture"].includes(r.name);
const materials = rows("media-materials", "media_materials.materials", "id,name,metadata")
  .filter(r => libraryFixture(r) && !r.metadata?.migration);
const products = rows("media-materials", "media_materials.products", "id,name,metadata")
  .filter(r => libraryFixture(r) && !r.metadata?.migration);
const vips = rows("media-materials", "media_materials.virtual_ips", "id,name").filter(libraryFixture);
const workspaces = rows("media-control", "workspaces", "id,name,organization_id")
  .filter(w => w.organization_id === org && /^(acl-multi-|ui-resume-mock-)/.test(w.name));
const plan = { retained_workflow: keep, organization: org, jobs: removedJobs, workflows: removedWorkflows,
  templates: removedTemplates, pushes, assets: removedAssets, materials, products, vips, workspaces };
const planPath = join(backup, values.apply ? "cleanup-applied-plan.json" : "cleanup-inventory.json");
writeFileSync(planPath, JSON.stringify(plan, null, 2), { flag: "wx", mode: 0o600 });
console.log(JSON.stringify(Object.fromEntries(Object.entries(plan).map(([k, v]) => [k, Array.isArray(v) ? v.length : v]))));
if (!values.apply) process.exit(0);
command(["bash", "harness/tools/verify_database_backups.sh", backup]);
const services = ["media_workflow-media_workflow-1", "media_workflow-internal_worker-1", "media_materials-media_materials-1"];
command(["docker", "stop", ...services]);
try {
  // Recheck target IDs after writers stop. Never sweep records created later.
  const tuples = removedJobs.map(j => `('${j.id}','${j.workflow_id}')`).join(",");
  if (tuples && Number(sql("media_workflow", `SELECT count(*) FROM media_workflow.jobs WHERE (id,workflow_id) IN (${tuples});`).trim()) !== removedJobs.length)
    throw new Error("Inventory changed before maintenance");
  const visited = new Set();
  async function removeEngine(id) {
    if (!id || visited.has(id)) return;
    if (!/^[a-f0-9-]{36}$/.test(id)) throw new Error("Unexpected engine instance ID");
    visited.add(id);
    const url = `http://127.0.0.1:8080/api/workflow/${id}`;
    const r = await fetch(url, { signal: AbortSignal.timeout(30000) });
    if (r.status === 404) return;
    if (!r.ok) throw new Error(`Engine lookup failed (${r.status})`);
    const state = await r.json();
    const children = (state.tasks || []).map(t => t.subWorkflowId || t.outputData?.subWorkflowId).filter(Boolean);
    const terminal = ["COMPLETED", "FAILED", "TIMED_OUT", "TERMINATED"].includes(state.status);
    const route = terminal ? "/remove?archiveWorkflow=false" : "/terminate-remove?archiveWorkflow=false&reason=backed-up-test-data-cleanup";
    const deleted = await fetch(url + route, { method: "DELETE", signal: AbortSignal.timeout(30000) });
    if (!deleted.ok && deleted.status !== 404) throw new Error(`Engine cleanup failed (${deleted.status})`);
    for (const child of children) await removeEngine(child);
  }
  for (const j of removedJobs) await removeEngine(j.conductor_workflow_id);
  writeFileSync(join(backup, "removed-engine-instances.json"), JSON.stringify([...visited]), { flag: "wx", mode: 0o600 });
  const j = ids(removedJobs), w = ids(removedWorkflows), t = ids(removedTemplates);
  sql("media_workflow", `BEGIN;
    SET LOCAL lock_timeout='10s';
    DELETE FROM media_workflow.historical_video_pushes WHERE id IN (${ids(pushes)});
    DELETE FROM media_workflow.step_run_reviews WHERE job_id IN (${j});
    DELETE FROM media_workflow.resume_outbox WHERE job_id IN (${j});
    DELETE FROM media_workflow.execution_starts WHERE job_id IN (${j});
    DELETE FROM media_workflow.job_submissions WHERE job_id IN (${j});
    DELETE FROM media_workflow.jobs WHERE id IN (${j});
    ALTER TABLE media_workflow.workflow_publications DISABLE TRIGGER immutable_workflow_publication;
    DELETE FROM media_workflow.workflow_publications WHERE workflow_id IN (${w});
    ALTER TABLE media_workflow.workflow_publications ENABLE TRIGGER immutable_workflow_publication;
    ALTER TABLE media_workflow.workflow_versions DISABLE TRIGGER frozen_workflow_version;
    DELETE FROM media_workflow.workflow_versions WHERE workflow_id IN (${w});
    ALTER TABLE media_workflow.workflow_versions ENABLE TRIGGER frozen_workflow_version;
    DELETE FROM media_workflow.workflows WHERE id IN (${w});
    DELETE FROM media_workflow.templates WHERE id IN (${t});
    COMMIT;`);
  const a = ids(removedAssets), m = ids(materials), p = ids(products), v = ids(vips);
  sql("media-materials", `BEGIN;
    SET LOCAL lock_timeout='10s';
    ALTER TABLE media_materials.generated_asset_results DISABLE TRIGGER generation_result_receipt_immutable;
    DELETE FROM media_materials.generated_asset_results WHERE asset_id IN (${a});
    ALTER TABLE media_materials.generated_asset_results ENABLE TRIGGER generation_result_receipt_immutable;
    DELETE FROM media_materials.generated_assets WHERE id IN (${a});
    DELETE FROM media_materials.material_bindings WHERE material_id IN (${m}) OR
      (owner_kind='product' AND owner_id IN (${p})) OR (owner_kind='virtual_ip' AND owner_id IN (${v}));
    DELETE FROM media_materials.materials WHERE id IN (${m});
    DELETE FROM media_materials.products WHERE id IN (${p});
    DELETE FROM media_materials.virtual_ips WHERE id IN (${v});
    COMMIT;`);
  sql("media-control", `BEGIN; DELETE FROM workspaces WHERE id IN (${ids(workspaces)}); COMMIT;`);
  console.log(`Cleanup committed; removed ${visited.size} selected engine instances. Shared object files, authentication and unrelated workspaces retained.`);
} finally {
  command(["docker", "start", ...services]);
}
