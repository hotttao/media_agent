import { loadEnvFile } from "node:process";
import { readFileSync } from "node:fs";
import { performance } from "node:perf_hooks";

// Read-only baseline. Credentials/private payloads are never printed.
loadEnvFile("ui_workflow/e2e/.env");
const base = process.env.PERF_GATEWAY_URL || "http://192.168.2.41:18080";
const authMode = process.env.PERF_AUTH_MODE || "token";
const rounds = Number(process.env.PERF_ROUNDS || "3");
if (!Number.isInteger(rounds) || rounds < 1 || rounds > 20) throw new Error("Rounds must be 1..20");
let headers;
if (authMode === "cookie") {
  const state = JSON.parse(readFileSync("ui_workflow/e2e/auth/storage-state.json"));
  const host = new URL(base).hostname;
  headers = { Cookie: state.cookies.filter((c) => host === c.domain.replace(/^\./, "") || host.endsWith(c.domain))
    .map((c) => `${c.name}=${c.value}`).join("; ") };
  if (!headers.Cookie) throw new Error("Matching browser session required");
} else if (authMode === "token" && process.env.E2E_API_TOKEN) headers = { Authorization: `Bearer ${process.env.E2E_API_TOKEN}` };
else throw new Error("Valid auth mode and test credential required");
async function get(path) {
  const start = performance.now();
  const r = await fetch(base + path, { headers, signal: AbortSignal.timeout(30000), redirect: "manual" });
  const headerMs = performance.now() - start;
  const body = await r.arrayBuffer();
  const ms = performance.now() - start;
  let data;
  try { data = JSON.parse(new TextDecoder().decode(body)); } catch { /* no private diagnostics */ }
  return { status: r.status, ms, headerMs, bytes: body.byteLength, data };
}
const me = await get("/api/auth/me");
if (me.status !== 200) throw new Error(`Authenticated baseline preflight status=${me.status}`);
const workspaces = await get("/api/v1/workspaces");
const workspace = workspaces.data?.workspaces?.[0];
if (!workspace?.organization_id) throw new Error("Owned Workspace required");
const org = workspace.organization_id;
const root = `/api/v1`;
const completedJobID = process.env.PERF_COMPLETED_JOB_ID || process.env.E2E_SAMPLE_JOB_ID || "job_e8a850ec-20dd-445f-b7cd-e89fe1b35592";
const cases = [
  ["auth.me", "/api/auth/me"],
  ["control.workspaces", `${root}/workspaces`],
  ["materials.page20", `${root}/materials?organization_id=${org}&scope=ai_clothing&page_size=20`],
  ["materials.page200", `${root}/materials?organization_id=${org}&scope=ai_clothing&page_size=200`],
  ["products.page20", `${root}/products?organization_id=${org}&scope=ai_clothing&page_size=20`],
  ["products.page200", `${root}/products?organization_id=${org}&scope=ai_clothing&page_size=200`],
  ["virtual_ips.page20", `${root}/virtual-ips?organization_id=${org}&scope=ai_clothing&page_size=20`],
  ["generated_assets.page20", `${root}/generated-assets?organization_id=${org}&scope=ai_clothing&page_size=20`],
  ["generated_assets.page200", `${root}/generated-assets?organization_id=${org}&scope=ai_clothing&page_size=200`],
  ["templates.page20", `${root}/templates?page_size=20`],
  ["workflows.page20", `${root}/workflows?page_size=20`],
  ["capabilities.page200", `${root}/capabilities?page_size=200`],
  ["jobs.page20", `${root}/jobs?page_size=20`],
  ["jobs.page200", `${root}/jobs?page_size=200`],
  ["jobs.completed", `${root}/jobs/${encodeURIComponent(completedJobID)}`],
  ["workflow.detail", `${root}/workflows/wf_2c1231e5-7379-4207-a0de-a9bb5d93b278`],
];
const selected = process.env.PERF_CASES?.split(",");
if (selected?.some((name) => !cases.some(([known]) => known === name))) throw new Error("Unknown performance case");
for (const [name, path] of cases.filter(([name]) => !selected || selected.includes(name))) {
  const measurements = [];
  for (let i = 0; i < rounds; i++) measurements.push(await get(path));
  const sorted = measurements.map((r) => r.ms).sort((a, b) => a - b);
  console.log(JSON.stringify({ name, auth: authMode, rounds, statuses: measurements.map((r) => r.status),
    median_ms: Math.round(sorted[Math.floor(sorted.length / 2)]), max_ms: Math.round(sorted.at(-1)),
    median_headers_ms: Math.round(measurements.map((r) => r.headerMs).sort((a, b) => a - b)[Math.floor(rounds / 2)]),
    bytes: measurements.at(-1).bytes, items: measurements.at(-1).data?.items?.length }));
  if (measurements.some((r) => r.status !== 200)) throw new Error(`Invalid benchmark: ${name} did not return 200`);
}
