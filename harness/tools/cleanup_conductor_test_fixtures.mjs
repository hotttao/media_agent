// Isolated E2E executions have no business Job rows; provenance is their exact test-name contract.
import { parseArgs } from "node:util";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join, isAbsolute } from "node:path";
const { values } = parseArgs({ options: { "backup-dir": { type: "string" }, apply: { type: "boolean", default: false } } });
const backup = values["backup-dir"];
if (!backup || !isAbsolute(backup) || !existsSync(join(backup, "conductor.db"))) throw new Error("Conductor backup required");
JSON.parse(readFileSync(join(backup, "cleanup-applied-plan.json"), "utf8"));
const fixture = /^(managed_e2e_|media_workflow_review_e2e_|regen_e2e_|review_delivery_|parallel_rerun_|start_idempotency_|start_loss_|start_delivery_|start_restart_|retry_delivery_|retry_marker_|media_workflow_native_retry_|media_workflow_finalize_flow_|provider_failure_|input_rerun_)[a-f0-9-]{20,36}$/;
const base = "http://127.0.0.1:8080/api";
async function get(path) {
  const r = await fetch(base + path, { signal: AbortSignal.timeout(30000) });
  if (!r.ok) throw new Error(`Conductor lookup failed (${r.status})`);
  return r.json();
}
const executions = [];
for (let start = 0; ; start += 200) {
  const page = await get(`/workflow/search?start=${start}&size=200`);
  executions.push(...page.results.filter(x => fixture.test(x.workflowType)));
  if (start + page.results.length >= page.totalHits || !page.results.length) break;
}
console.log(JSON.stringify({ isolated_test_executions: executions.length }));
if (!values.apply) process.exit(0);
writeFileSync(join(backup, "removed-isolated-engine-tests.json"), JSON.stringify(executions), { flag: "wx", mode: 0o600 });
let next = 0;
await Promise.all(Array.from({ length: 4 }, async () => {
  while (next < executions.length) {
    const w = executions[next++];
    if (!/^[a-f0-9-]{36}$/.test(w.workflowId)) throw new Error("Invalid engine instance ID");
    const terminal = ["COMPLETED", "FAILED", "TIMED_OUT", "TERMINATED"].includes(w.status);
    const route = terminal ? "remove" : "terminate-remove";
    const r = await fetch(`${base}/workflow/${w.workflowId}/${route}?archiveWorkflow=false&reason=backed-up-isolated-test-cleanup`,
      { method: "DELETE", signal: AbortSignal.timeout(30000) });
    if (!r.ok && r.status !== 404) throw new Error(`Test instance cleanup failed (${r.status})`);
  }
}));
const definitions = await get("/metadata/workflow");
const legacyTasks = new Set(["media_content_ai_fill_v1", "media_publish_feishu_v1", "media_image_model_v1",
  "media_image_style_v1", "media_image_first_frame_v1", "media_video_generate_v1", "media_video_clip_v1"]);
const tasks = (await get("/metadata/taskdefs")).filter(t => legacyTasks.has(t.name));
for (const t of tasks)
  if (definitions.some(d => JSON.stringify(d.tasks).includes(`"${t.name}"`))) throw new Error("Legacy task still referenced by retained workflow");
writeFileSync(join(backup, "removed-legacy-engine-task-definitions.json"), JSON.stringify(tasks), { flag: "wx", mode: 0o600 });
for (const t of tasks) {
  const r = await fetch(`${base}/metadata/taskdefs/${t.name}`, { method: "DELETE", signal: AbortSignal.timeout(30000) });
  if (!r.ok && r.status !== 404) throw new Error(`Legacy task cleanup failed (${r.status})`);
}
console.log(`Removed ${executions.length} isolated test instances and ${tasks.length} unreferenced legacy fixture task definitions.`);
