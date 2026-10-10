// Remove only engine definitions of workflow IDs in the applied maintenance manifest.
import { parseArgs } from "node:util";
import { readFileSync, writeFileSync } from "node:fs";
import { join, isAbsolute } from "node:path";
const { values } = parseArgs({ options: { "backup-dir": { type: "string" }, apply: { type: "boolean", default: false } } });
const backup = values["backup-dir"];
if (!backup || !isAbsolute(backup)) throw new Error("Private backup directory required");
const plan = JSON.parse(readFileSync(join(backup, "cleanup-applied-plan.json"), "utf8"));
const removed = plan.workflows.map(w => w.id.replaceAll("-", "_"));
const r = await fetch("http://127.0.0.1:8080/api/metadata/workflow", { signal: AbortSignal.timeout(30000) });
if (!r.ok) throw new Error("Engine metadata lookup failed");
const selected = (await r.json()).filter(w =>
  w.name.startsWith("media_workflow_") && removed.some(id => w.name === `media_workflow_${id}` ||
    w.name.startsWith(`media_workflow_resume_${id}_`)));
console.log(JSON.stringify({ selected_definitions: selected.length }));
if (!values.apply) process.exit(0);
writeFileSync(join(backup, "removed-engine-definitions.json"), JSON.stringify(selected), { flag: "wx", mode: 0o600 });
for (const w of selected) {
  if (!Number.isInteger(w.version) || w.version < 1) throw new Error("Invalid engine definition version");
  const d = await fetch(`http://127.0.0.1:8080/api/metadata/workflow/${encodeURIComponent(w.name)}/${w.version}`,
    { method: "DELETE", signal: AbortSignal.timeout(30000) });
  if (!d.ok && d.status !== 404) throw new Error(`Engine definition cleanup failed (${d.status})`);
}
console.log(`Removed ${selected.length} selected definitions; shared task definitions preserved.`);
