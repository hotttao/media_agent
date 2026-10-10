// Run from the repository root after scoped reset. Secrets stay in process environments.
import { loadEnvFile } from "node:process";
import { parseArgs } from "node:util";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { openSync, closeSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { join, isAbsolute } from "node:path";
const { values } = parseArgs({ options: {
  "backup-dir": { type: "string" }, "workflow-id": { type: "string" },
  "workspace-id": { type: "string" }, "organization-id": { type: "string" },
  "owner-user-id": { type: "string" }, "verify-only": { type: "boolean", default: false },
} });
const backup = values["backup-dir"];
if (!backup || !isAbsolute(backup) || !existsSync(join(backup, "SHA256SUMS"))) throw new Error("Private snapshot directory required");
for (const k of ["workflow-id", "workspace-id", "organization-id", "owner-user-id"])
  if (!/^[a-zA-Z0-9_-]+$/.test(values[k] || "")) throw new Error(`Explicit ${k} required`);
loadEnvFile("/home/tao/code/github/media_ai/.env");
loadEnvFile("ui_workflow/e2e/.env");
if (!process.env.DATABASE_URL || !process.env.E2E_API_TOKEN) throw new Error("Existing source credentials and User API Token required");
const env = { ...process.env,
  MEDIA_AI_DATABASE_URL: process.env.DATABASE_URL,
  MEDIA_MATERIALS_BEARER_TOKEN: process.env.E2E_API_TOKEN,
  MEDIA_MATERIALS_ORGANIZATION_ID: values["organization-id"],
  MEDIA_MATERIALS_OWNER_USER_ID: values["owner-user-id"],
  MEDIA_MATERIALS_URL: "http://192.168.2.41:18080",
  MEDIA_GATEWAY_ADDR: "http://192.168.2.41:18080",
  MEDIA_MATERIALS_MIGRATION_TOKEN: randomBytes(32).toString("hex"),
  MEDIA_WORKFLOW_MIGRATION_TOKEN: randomBytes(32).toString("hex"),
};
const attempt = `${Date.now()}-${process.pid}`;
async function run(cmd, args, name, accepted = [0]) {
  const path = join(backup, `${name}-${attempt}.log`);
  const fd = openSync(path, "wx", 0o600);
  const status = await new Promise((resolve, reject) => {
    const p = spawn(cmd, args, { env, stdio: ["ignore", fd, fd] });
    p.on("error", reject); p.on("exit", c => resolve(c));
  }).finally(() => closeSync(fd));
  if (!accepted.includes(status)) throw new Error(`${name} failed (${status}); private log retained`);
  console.log(`${name}: exit=${status}`);
  return { status, path };
}
async function compose(service, name) {
  await run("docker", ["compose", "-f", `${service}/docker-compose.yml`, "up", "-d", "--no-build", "--no-deps", "--force-recreate", service], name);
}
async function ready() {
  for (let n = 0; n < 30; n++) {
    const ok = await Promise.all([8002, 8003].map(async port => {
      try { return (await fetch(`http://127.0.0.1:${port}/health`, { signal: AbortSignal.timeout(3000) })).ok; }
      catch { return false; }
    }));
    if (ok.every(Boolean)) return;
    await new Promise(r => setTimeout(r, 1000));
  }
  throw new Error("Target APIs did not become healthy");
}
const flags = ["--organization-id", values["organization-id"], "--owner-user-id", values["owner-user-id"],
  "--workspace-id", values["workspace-id"], "--workflow-id", values["workflow-id"], "--workflow-version", "1"];
const report = join(backup, "history-reimport.jsonl");
if (!values["verify-only"]) {
  try {
    await compose("media_materials", "enable-materials-import");
    await compose("media_workflow", "enable-workflow-import");
    await ready();
    await run("/tmp/media-agent-materials-reimport", [], "materials-reimport");
    const result = await run("/tmp/media-agent-history-reimport", [...flags, "--import-all-report", report], "history-reimport", [0, 1]);
    const rows = readFileSync(report, "utf8").trim().split("\n").map(s => JSON.parse(s));
    const summary = Object.create(null);
    for (const r of rows) summary[`${r.kind}:${r.status}`] = (summary[`${r.kind}:${r.status}`] || 0) + 1;
    writeFileSync(join(backup, "reimport-summary.json"), JSON.stringify(summary, null, 2), { flag: "wx", mode: 0o600 });
    console.log(JSON.stringify(summary));
    const failures = rows.filter(r => r.status === "failed");
    // Only the already-audited missing source file is an expected quarantine.
    if (failures.some(r => r.kind !== "video_push" || r.source_id !== "d8ef3ac8-32c9-414b-bc5d-1c1eeadca214") ||
        rows.filter(r => r.kind === "incomplete_group" && r.status === "imported").length !== 121 ||
        rows.filter(r => r.kind === "video" && r.status === "imported").length !== 125 ||
        rows.filter(r => r.kind === "video_push" && r.status === "imported").length !== 52)
      throw new Error("Reimport did not meet the audited source inventory; inspect private report");
    if (result.status !== 0 && failures.length !== 1) throw new Error("Unexpected nonzero import result");
  } finally {
    env.MEDIA_MATERIALS_MIGRATION_TOKEN = "";
    env.MEDIA_WORKFLOW_MIGRATION_TOKEN = "";
    // Try both shutdown operations even if one service fails to recreate.
    const errors = [];
    for (const service of ["media_materials", "media_workflow"])
      try { await compose(service, `disable-${service}-import`); } catch (e) { errors.push(e); }
    if (errors.length) throw new Error("Migration entry shutdown failed; inspect private compose logs");
    await ready();
  }
}
const verify = await run("/tmp/media-agent-history-reimport", [...flags, "--verify-report", report], "history-verify", [0, 1]);
const evidence = readFileSync(verify.path, "utf8").match(/reconciliation verified: imported=(\d+) excluded=(\d+) quarantined=(\d+) unique_objects=(\d+)/);
if (!evidence || evidence[1] !== "298" || evidence[2] !== "50" || evidence[3] !== "1" || evidence[4] !== "538")
  throw new Error("Read-only reconciliation did not verify all expected records and objects");
console.log("Verified 298 imported records, 50 exclusions and 538 readable objects; one known missing source publication remains quarantined.");
