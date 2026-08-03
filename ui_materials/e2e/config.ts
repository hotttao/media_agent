import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { load as loadYaml } from "js-yaml";

// ── Directory resolution ──

const E2E_DIR = dirname(fileURLToPath(import.meta.url));

// ── Types ──

/**
 * Minimal E2E config. Add platform-specific sections here as the project
 * grows (e.g. storage backends, model registries, third-party auth providers).
 */
export interface E2eConfig {
  auth: { email: string; password: string };
}

// ── Profile loading ──
// Set E2E_PROFILE_PATH to an absolute path, or E2E_PROFILE to load from e2e/profiles/{name}.yaml

let profilePath: string;

if (process.env.E2E_PROFILE_PATH) {
  profilePath = process.env.E2E_PROFILE_PATH;
} else {
  const profileName = process.env.E2E_PROFILE || "default";
  profilePath = resolve(E2E_DIR, `profiles/${profileName}.yaml`);
}

if (!existsSync(profilePath)) {
  throw new Error(
    `E2E profile not found at ${profilePath}.\nSet E2E_PROFILE_PATH or E2E_PROFILE to a valid profile.`,
  );
}

// biome-ignore lint/suspicious/noExplicitAny: YAML profile values are dynamically typed
const raw: Record<string, any> =
  (loadYaml(readFileSync(profilePath, "utf-8")) as Record<string, unknown>) ??
  {};

// biome-ignore lint/suspicious/noExplicitAny: raw YAML sections are untyped
function buildConfig(raw: Record<string, any>): E2eConfig {
  const auth = raw.auth ?? {};
  return {
    auth: {
      email: auth.email ?? "admin@example.com",
      password: auth.password ?? "admin",
    },
  };
}

export const config: E2eConfig = buildConfig(raw);
