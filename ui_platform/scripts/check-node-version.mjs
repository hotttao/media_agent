#!/usr/bin/env node
/**
 * Preflight guard for `predev` / `prebuild`.
 *
 * Vite 6 requires Node.js >= 18. Failing here is cheaper than letting
 * `vite` crash mid-start with a cryptic "Unexpected token export" or
 * similar when an old Node hits ESM-only dependencies.
 *
 * Also warns on Node 22+ that some legacy native modules (e.g.
 * `node-sass`) emit deprecation notices — purely informational.
 */

const REQUIRED_MAJOR = 18;
const current = process.versions.node;
const major = Number.parseInt(current.split(".")[0] ?? "0", 10);

if (Number.isNaN(major) || major < REQUIRED_MAJOR) {
	console.error(
		`\u274c Node.js ${REQUIRED_MAJOR}+ required, found ${current}.\n` +
			`   Upgrade via nvm: nvm install ${REQUIRED_MAJOR} && nvm use ${REQUIRED_MAJOR}\n`,
	);
	process.exit(1);
}

console.log(`\u2705 Node.js ${current} \u2265 ${REQUIRED_MAJOR} — preflight passed`);